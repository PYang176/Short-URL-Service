package trial

import (
	"context"
	"log/slog"
	"time"
)

const (
	// cleanupBatchSize 是单次事务最多清理的游客工作空间数量
	cleanupBatchSize = 100
)

// Cleaner 定期清理过期游客会话和关联链接
type Cleaner struct {
	// repo 提供游客数据清理能力
	repo Repository
	// logger 是结构化日志
	logger *slog.Logger
	// interval 是两轮清理之间的等待时间
	interval time.Duration
	// now 提供可替换的 UTC 时间
	now func() time.Time
}

// NewCleaner 创建游客数据清理任务
func NewCleaner(repo Repository, logger *slog.Logger, interval time.Duration) *Cleaner {
	return &Cleaner{
		repo:     repo,
		logger:   logger,
		interval: interval,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

// Run 在 Worker 生命周期内定时执行游客清理
func (c *Cleaner) Run(ctx context.Context) {
	c.cleanup(ctx)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.cleanup(ctx)
		}
	}
}

// cleanup 连续处理批次，直到没有更多过期会话或上下文取消
func (c *Cleaner) cleanup(ctx context.Context) {
	for ctx.Err() == nil {
		deleted, err := c.repo.CleanupExpired(ctx, c.now(), cleanupBatchSize)
		if err != nil {
			if ctx.Err() == nil {
				c.logger.Warn("清理过期游客数据失败", "error", err)
			}
			return
		}
		if deleted > 0 {
			c.logger.Info("已清理过期游客工作空间", "count", deleted)
		}
		if deleted < cleanupBatchSize {
			return
		}
	}
}
