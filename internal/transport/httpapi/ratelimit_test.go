package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/PYang176/Short-URL-Service/internal/platform/config"
	"github.com/PYang176/Short-URL-Service/internal/platform/ratelimit"
)

// TestRateLimitByIP 验证超过阈值时返回 429 和 Retry-After
func TestRateLimitByIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("关闭 Redis 失败: %v", err)
		}
	})

	limiter := ratelimit.New(client)
	router := gin.New()
	router.Use(
		RequestID(),
		RateLimitByIP(
			limiter,
			slog.New(slog.NewTextHandler(io.Discard, nil)),
			"test",
			config.RateLimitRule{Limit: 1, Window: time.Minute},
		),
	)
	router.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/test", nil))
	if first.Code != http.StatusNoContent {
		t.Fatalf("first status = %d, want 204", first.Code)
	}
	second := httptest.NewRecorder()
	router.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/test", nil))
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want 429", second.Code)
	}
	if second.Header().Get("Retry-After") == "" {
		t.Fatal("Retry-After is empty")
	}
}

// TestRateLimitIgnoresClientRequestID 验证客户端固定 Request ID 不能绕过限流
func TestRateLimitIgnoresClientRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("关闭 Redis 失败: %v", err)
		}
	})

	limiter := ratelimit.New(client)
	router := gin.New()
	router.Use(
		RequestID(),
		RateLimitByIP(
			limiter,
			slog.New(slog.NewTextHandler(io.Discard, nil)),
			"fixed-request-id",
			config.RateLimitRule{Limit: 1, Window: time.Minute},
		),
	)
	router.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	for attempt := 0; attempt < 2; attempt++ {
		request := httptest.NewRequest(http.MethodGet, "/test", nil)
		request.Header.Set("X-Request-ID", "client-controlled-id")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		want := http.StatusNoContent
		if attempt == 1 {
			want = http.StatusTooManyRequests
		}
		if response.Code != want {
			t.Fatalf("attempt %d status = %d, want %d", attempt+1, response.Code, want)
		}
	}
}
