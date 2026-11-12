package ratelimit

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/PYang176/Short-URL-Service/internal/platform/config"
)

const slidingWindowScript = `
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local member = ARGV[4]

redis.call('ZREMRANGEBYSCORE', key, '-inf', now - window)
local count = redis.call('ZCARD', key)
if count >= limit then
	local oldest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
	local retry = 1
	if oldest[2] then
		retry = math.max(1, math.ceil((tonumber(oldest[2]) + window - now) / 1000))
	end
	return {0, count, retry}
end

redis.call('ZADD', key, now, member)
redis.call('PEXPIRE', key, window + 1000)
return {1, count + 1, 0}
`

// Decision 是单次限流判断结果
type Decision struct {
	// Allowed 表示请求是否允许
	Allowed bool
	// Count 是当前窗口内已使用数量
	Count int
	// RetryAfter 是建议重试等待时间
	RetryAfter time.Duration
}

// Limiter 使用 Redis ZSET 实现滑动窗口限流
type Limiter struct {
	// redis 是共享限流状态存储
	redis *redis.Client
	// now 提供可替换的当前时间，测试可控制滑动窗口
	now func() time.Time
}

// New 创建 Redis 滑动窗口限流器
func New(redisClient *redis.Client) *Limiter {
	return &Limiter{
		redis: redisClient,
		now:   func() time.Time { return time.Now() },
	}
}

// Allow 判断指定 Key 在规则下是否放行
func (l *Limiter) Allow(
	ctx context.Context,
	key, member string,
	rule config.RateLimitRule,
) (Decision, error) {
	now := l.now().UnixMilli()
	windowMillis := rule.Window.Milliseconds()
	result, err := l.redis.Eval(ctx, slidingWindowScript, []string{key},
		now,
		windowMillis,
		rule.Limit,
		member,
	).Result()
	if err != nil {
		return Decision{}, fmt.Errorf("执行 Redis 限流脚本失败: %w", err)
	}

	values, ok := result.([]any)
	if !ok || len(values) != 3 {
		return Decision{}, fmt.Errorf("redis 限流脚本返回格式无效")
	}
	allowed, err := strconv.ParseInt(fmt.Sprint(values[0]), 10, 64)
	if err != nil {
		return Decision{}, fmt.Errorf("解析限流结果失败: %w", err)
	}
	count, err := strconv.ParseInt(fmt.Sprint(values[1]), 10, 64)
	if err != nil {
		return Decision{}, fmt.Errorf("解析限流计数失败: %w", err)
	}
	retrySeconds, err := strconv.ParseInt(fmt.Sprint(values[2]), 10, 64)
	if err != nil {
		return Decision{}, fmt.Errorf("解析限流重试时间失败: %w", err)
	}
	return Decision{
		Allowed:    allowed == 1,
		Count:      int(count),
		RetryAfter: time.Duration(retrySeconds) * time.Second,
	}, nil
}
