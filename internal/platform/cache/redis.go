package cache

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/PYang176/Short-URL-Service/internal/platform/config"
)

// OpenRedis 创建 Redis 客户端并验证连接可用
func OpenRedis(ctx context.Context, cfg config.RedisConfig) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis 连通性检查失败: %w", err)
	}
	return client, nil
}
