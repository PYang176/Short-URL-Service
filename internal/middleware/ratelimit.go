package middleware

import (
	"context"
	"log"
	"net/http"
	"shortURL/internal/config"
	"shortURL/internal/redis"
	"time"
)

// RedisRateLimit 基于 Redis 的限流中间件
func RedisRateLimit(cfg config.RouteLimitConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 以 IP 作为限流 Key
			key := "ratelimit:" + r.RemoteAddr + ":" + r.URL.Path
			ctx := context.Background()

			// 使用 Lua 脚本保证原子性（防止并发超卖）
			script := `
				local key = KEYS[1]
				local limit = tonumber(ARGV[1])
				local window = tonumber(ARGV[2])  -- 秒数
				
				local current = redis.call('INCR', key)
				if current == 1 then
					redis.call('EXPIRE', key, window)  -- 使用 EXPIRE，单位是秒
				end
				
				if current > limit then
					return 0
				end
				return 1
			`

			res, err := redis.RDB.Eval(
				ctx,
				script,
				[]string{key},
				cfg.Limit,
				time.Duration(cfg.WindowSec)*time.Second,
			).Int()
			if err != nil {
				log.Printf("[ERROR] Redis rate limit failed: %v", err)
				// Redis 挂了，放行
				next.ServeHTTP(w, r)
				return
			}

			if res == 0 {
				log.Printf("[WARN] Redis rate limit exceeded for %s", r.RemoteAddr)
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
