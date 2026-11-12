package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/PYang176/Short-URL-Service/internal/platform/config"
)

// TestLimiterSlidingWindow 验证窗口内超限和过期后恢复
func TestLimiterSlidingWindow(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("关闭 Redis 失败: %v", err)
		}
	})

	limiter := New(client)
	current := time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return current }
	rule := config.RateLimitRule{
		Limit:      2,
		Window:     2 * time.Second,
		FailClosed: false,
	}
	for index := 0; index < 2; index++ {
		decision, err := limiter.Allow(context.Background(), "limit-test", "request-"+string(rune('a'+index)), rule)
		if err != nil || !decision.Allowed {
			t.Fatalf("Allow() request %d = %+v, %v", index, decision, err)
		}
	}
	decision, err := limiter.Allow(context.Background(), "limit-test", "request-c", rule)
	if err != nil {
		t.Fatalf("Allow() over limit error = %v", err)
	}
	if decision.Allowed || decision.RetryAfter <= 0 {
		t.Fatalf("Allow() over limit = %+v", decision)
	}

	current = current.Add(3 * time.Second)
	decision, err = limiter.Allow(context.Background(), "limit-test", "request-d", rule)
	if err != nil || !decision.Allowed {
		t.Fatalf("Allow() after window = %+v, %v", decision, err)
	}
}
