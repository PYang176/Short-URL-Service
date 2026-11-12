package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// workerBatchSize 是单次最多领取的事件数量
	workerBatchSize = 100
	// pendingIdle 是其他消费者遗留消息允许等待的最长时间
	pendingIdle = 30 * time.Second
)

// Worker 消费 Redis Stream 并批量写入 MySQL
type Worker struct {
	// repo 是点击事件存储
	repo Repository
	// redis 是点击事件 Stream 客户端
	redis *redis.Client
	// logger 是结构化日志
	logger *slog.Logger
	// stream 是 Redis Stream 名称
	stream string
	// group 是消费者组名称
	group string
	// consumer 是当前进程消费者名称
	consumer string
}

// NewWorker 创建点击事件消费者
func NewWorker(
	repo Repository,
	redisClient *redis.Client,
	logger *slog.Logger,
	stream, group string,
) *Worker {
	hostname, _ := os.Hostname()
	return &Worker{
		repo:     repo,
		redis:    redisClient,
		logger:   logger,
		stream:   stream,
		group:    group,
		consumer: fmt.Sprintf("%s-%d", hostname, os.Getpid()),
	}
}

// Run 持续消费新事件并接管超时的 Pending 消息
func (w *Worker) Run(ctx context.Context) error {
	if err := w.ensureGroup(ctx); err != nil {
		return err
	}

	w.logger.Info("点击分析 Worker 启动",
		"stream", w.stream,
		"group", w.group,
		"consumer", w.consumer,
	)

	for {
		if ctx.Err() != nil {
			return nil
		}

		streams, err := w.redis.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    w.group,
			Consumer: w.consumer,
			Streams:  []string{w.stream, ">"},
			Count:    workerBatchSize,
			Block:    time.Second,
		}).Result()
		if err != nil && !errors.Is(err, redis.Nil) && ctx.Err() == nil {
			w.logger.Warn("读取点击事件失败", "error", err)
			time.Sleep(time.Second)
		}
		if err == nil {
			for _, stream := range streams {
				w.processMessages(ctx, stream.Messages)
			}
		}

		claimed, _, claimErr := w.redis.XAutoClaim(ctx, &redis.XAutoClaimArgs{
			Stream:   w.stream,
			Group:    w.group,
			Consumer: w.consumer,
			MinIdle:  pendingIdle,
			Start:    "0-0",
			Count:    workerBatchSize,
		}).Result()
		if claimErr != nil && !errors.Is(claimErr, redis.Nil) && ctx.Err() == nil {
			w.logger.Warn("接管 Pending 点击事件失败", "error", claimErr)
		}
		if claimErr == nil && len(claimed) > 0 {
			w.processMessages(ctx, claimed)
		}
	}
}

// ensureGroup 创建消费者组；组已存在时直接复用
func (w *Worker) ensureGroup(ctx context.Context) error {
	err := w.redis.XGroupCreateMkStream(ctx, w.stream, w.group, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return fmt.Errorf("创建点击事件消费者组失败: %w", err)
	}
	return nil
}

// processMessages 解析并批量写入消息，成功后 ACK
func (w *Worker) processMessages(ctx context.Context, messages []redis.XMessage) {
	if len(messages) == 0 {
		return
	}

	events := make([]Event, 0, len(messages))
	ackIDs := make([]string, 0, len(messages))
	for _, message := range messages {
		raw, ok := message.Values["event"].(string)
		if !ok {
			w.logger.Warn("点击消息缺少 event 字段", "stream_id", message.ID)
			ackIDs = append(ackIDs, message.ID)
			continue
		}

		var event Event
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			w.logger.Warn("解析点击事件失败", "stream_id", message.ID, "error", err)
			ackIDs = append(ackIDs, message.ID)
			continue
		}
		events = append(events, event)
		ackIDs = append(ackIDs, message.ID)
	}

	if len(events) > 0 {
		inserted, err := w.repo.InsertBatch(ctx, events)
		if err != nil {
			w.logger.Warn("批量写入点击事件失败，保留 Pending", "error", err)
			return
		}
		w.logger.Debug("点击事件批次已写入", "received", len(events), "inserted", inserted)
	}

	if len(ackIDs) > 0 {
		if err := w.redis.XAck(ctx, w.stream, w.group, ackIDs...).Err(); err != nil {
			w.logger.Warn("点击事件 ACK 失败", "error", err)
		}
	}
}
