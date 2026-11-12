package analytics

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// TestWorkerProcessesAndAcknowledgesMessages 验证 Worker 批量写入并 ACK
func TestWorkerProcessesAndAcknowledgesMessages(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		if err := redisClient.Close(); err != nil {
			t.Errorf("关闭 Redis 失败: %v", err)
		}
	})

	event := Event{
		EventID:     "event-1",
		LinkID:      "link-1",
		ClickedAt:   time.Now().UTC(),
		IsBot:       false,
		VisitorHash: []byte("visitor-hash"),
	}
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("序列化事件失败: %v", err)
	}
	if _, err := redisClient.XAdd(context.Background(), &redis.XAddArgs{
		Stream: "clicks",
		Values: map[string]any{"event": payload},
	}).Result(); err != nil {
		t.Fatalf("XAdd() error = %v", err)
	}

	repo := &fakeAnalyticsRepository{}
	worker := NewWorker(
		repo,
		redisClient,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		"clicks",
		"analytics",
	)
	if err := worker.ensureGroup(context.Background()); err != nil {
		t.Fatalf("ensureGroup() error = %v", err)
	}
	streams, err := redisClient.XReadGroup(context.Background(), &redis.XReadGroupArgs{
		Group:    "analytics",
		Consumer: "test",
		Streams:  []string{"clicks", ">"},
		Count:    10,
	}).Result()
	if err != nil {
		t.Fatalf("XReadGroup() error = %v", err)
	}
	worker.processMessages(context.Background(), streams[0].Messages)

	if len(repo.events) != 1 || repo.events[0].EventID != "event-1" {
		t.Fatalf("Worker 写入事件 = %+v", repo.events)
	}
	pending, err := redisClient.XPending(context.Background(), "clicks", "analytics").Result()
	if err != nil {
		t.Fatalf("XPending() error = %v", err)
	}
	if pending.Count != 0 {
		t.Fatalf("Pending 数量 = %d, want 0", pending.Count)
	}
}

type fakeAnalyticsRepository struct {
	mu     sync.Mutex
	events []Event
}

// InsertBatch 保存 Worker 测试事件
func (r *fakeAnalyticsRepository) InsertBatch(_ context.Context, events []Event) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, events...)
	return int64(len(events)), nil
}

// eventCount 返回已保存的测试事件数量
func (r *fakeAnalyticsRepository) eventCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.events)
}

// Summary 返回空的测试汇总
func (r *fakeAnalyticsRepository) Summary(context.Context, string, string, time.Time, time.Time) (Summary, error) {
	return Summary{}, nil
}

// Trend 返回空的测试趋势
func (r *fakeAnalyticsRepository) Trend(context.Context, string, string, time.Time, time.Time, bool) ([]TrendPoint, error) {
	return nil, nil
}

// Breakdown 返回空的测试维度
func (r *fakeAnalyticsRepository) Breakdown(context.Context, string, string, time.Time, time.Time, string) ([]BreakdownItem, error) {
	return nil, nil
}

// TestWorkerRecoversMissingConsumerGroup 验证 Stream 被清理后 Worker 自动重建消费组
func TestWorkerRecoversMissingConsumerGroup(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		if err := redisClient.Close(); err != nil {
			t.Errorf("关闭 Redis 失败: %v", err)
		}
	})

	repo := &fakeAnalyticsRepository{}
	worker := NewWorker(
		repo,
		redisClient,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		"clicks",
		"analytics",
	)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- worker.Run(ctx)
	}()
	time.Sleep(100 * time.Millisecond)

	if err := redisClient.Del(context.Background(), "clicks").Err(); err != nil {
		t.Fatalf("删除 Stream 失败: %v", err)
	}
	event := Event{
		EventID:     "event-recover",
		LinkID:      "link-1",
		ClickedAt:   time.Now().UTC(),
		VisitorHash: []byte("visitor-hash"),
	}
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("序列化事件失败: %v", err)
	}
	if _, err := redisClient.XAdd(context.Background(), &redis.XAddArgs{
		Stream: "clicks",
		Values: map[string]any{"event": payload},
	}).Result(); err != nil {
		t.Fatalf("XAdd() error = %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for repo.eventCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Worker.Run() error = %v", err)
	}
	if repo.eventCount() != 1 {
		t.Fatalf("Worker 恢复消费数量 = %d, want 1", repo.eventCount())
	}
}
