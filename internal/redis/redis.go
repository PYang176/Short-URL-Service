package redis

import (
	"context"
	"log"
	"shortURL/internal/config"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	RDB *redis.Client
	Ctx = context.Background()
)

// Init 初始化 Redis 连接
func Init(cfg *config.RedisConfig) error {
	RDB = redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
		// 连接池配置
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
	})

	// Ping 测试
	if err := RDB.Ping(Ctx).Err(); err != nil {
		return err
	}
	log.Println("Redis connection established")
	return nil
}
