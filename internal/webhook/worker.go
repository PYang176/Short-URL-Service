package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/PYang176/Short-URL-Service/internal/platform/config"
	"github.com/PYang176/Short-URL-Service/internal/platform/observability"
)

const (
	// deliveryBatchSize 是单轮最多并发投递的任务数
	deliveryBatchSize = 4
	// maxResponseDrainBytes 是读取响应体用于连接复用的上限
	maxResponseDrainBytes = 4 * 1024
)

// Worker 持续领取并投递 Webhook Outbox 任务
type Worker struct {
	// repo 是 Webhook 和 Outbox 仓库
	repo Repository
	// cipher 解密端点签名 Secret
	cipher *SecretCipher
	// client 是禁止重定向并执行 SSRF 检查的 HTTP 客户端
	client *http.Client
	// logger 是结构化日志
	logger *slog.Logger
	// cfg 控制轮询、超时、租约和重试次数
	cfg config.WebhookConfig
	// metrics 记录投递数量和耗时
	metrics *observability.WebhookMetrics
}

// NewWorker 创建 Webhook 投递 Worker
func NewWorker(
	repo Repository,
	cipher *SecretCipher,
	logger *slog.Logger,
	cfg config.WebhookConfig,
	metrics *observability.WebhookMetrics,
) *Worker {
	return &Worker{
		repo:    repo,
		cipher:  cipher,
		client:  NewHTTPClient(cfg.AllowPrivateTargets, cfg.RequestTimeout),
		logger:  logger,
		cfg:     cfg,
		metrics: metrics,
	}
}

// Run 按固定周期领取并处理 Webhook 投递任务
func (w *Worker) Run(ctx context.Context) error {
	w.logger.Info("Webhook Worker 启动", "poll_interval", w.cfg.PollInterval)
	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	for {
		if err := w.processDue(ctx); err != nil && ctx.Err() == nil {
			w.logger.Warn("处理 Webhook 投递任务失败", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// processDue 领取一批到期任务并受控并发投递
func (w *Worker) processDue(ctx context.Context) error {
	now := time.Now().UTC()
	jobs, err := w.repo.ClaimDeliveries(
		ctx,
		deliveryBatchSize,
		now,
		now.Add(w.cfg.Lease),
	)
	if err != nil {
		return err
	}
	var wait sync.WaitGroup
	wait.Add(len(jobs))
	for index := range jobs {
		go func(job *DeliveryJob) {
			defer wait.Done()
			if ctx.Err() != nil {
				return
			}
			w.deliver(ctx, job)
		}(&jobs[index])
	}
	wait.Wait()
	return nil
}

// deliver 发送单个 Webhook 并持久化最终结果
func (w *Worker) deliver(ctx context.Context, job *DeliveryJob) {
	started := time.Now()
	responseStatus, err := w.send(ctx, job)
	result := "success"
	if err != nil {
		result = "failure"
	}
	w.metrics.ObserveDelivery(job.EventType, result, time.Since(started))

	if err == nil {
		if markErr := w.repo.MarkSuccess(
			ctx,
			job,
			*responseStatus,
			time.Now().UTC(),
		); markErr != nil {
			w.logger.Warn("保存 Webhook 成功状态失败",
				"delivery_id", job.ID,
				"event_id", job.EventID,
				"error", markErr,
			)
		}
		return
	}

	if markErr := w.repo.MarkFailure(
		ctx,
		job,
		responseStatus,
		err.Error(),
		time.Now().UTC(),
		w.cfg.MaxAttempts,
	); markErr != nil {
		w.logger.Warn("保存 Webhook 失败状态失败",
			"delivery_id", job.ID,
			"event_id", job.EventID,
			"error", markErr,
		)
	}
	w.logger.Warn("Webhook 投递失败",
		"delivery_id", job.ID,
		"event_id", job.EventID,
		"event_type", job.EventType,
		"attempt", job.AttemptCount,
		"error", err,
	)
}

// send 使用 HMAC 签名发送事件，并仅把 2xx 视为成功
func (w *Worker) send(ctx context.Context, job *DeliveryJob) (*int, error) {
	secret, err := w.cipher.Decrypt(job.SecretCiphertext)
	if err != nil {
		return nil, fmt.Errorf("解密 Webhook Secret 失败: %w", err)
	}
	requestCtx, cancel := context.WithTimeout(ctx, w.cfg.RequestTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(
		requestCtx,
		http.MethodPost,
		job.EndpointURL,
		bytes.NewReader(job.Payload),
	)
	if err != nil {
		return nil, fmt.Errorf("创建 Webhook 请求失败: %w", err)
	}
	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	signature := signPayload(secret, timestamp, job.Payload)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "ShortURL-Webhook/1.0")
	request.Header.Set("X-ShortURL-Event-ID", job.EventID)
	request.Header.Set("X-ShortURL-Event-Type", job.EventType)
	request.Header.Set("X-ShortURL-Timestamp", timestamp)
	request.Header.Set("X-ShortURL-Signature", signature)

	response, err := w.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("发送 Webhook 请求失败: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseDrainBytes))
		_ = response.Body.Close()
	}()
	status := response.StatusCode
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return &status, fmt.Errorf("webhook 目标返回 HTTP %d", status)
	}
	return &status, nil
}

// signPayload 生成 timestamp.body 的 HMAC-SHA256 签名
func signPayload(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
