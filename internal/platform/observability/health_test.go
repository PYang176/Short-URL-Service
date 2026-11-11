package observability

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type stubChecker struct {
	err error
}

// Check 返回测试预设的依赖探测结果
func (c stubChecker) Check(context.Context) error {
	return c.err
}

// TestReadyDegradesWhenRedisIsUnavailable 验证 Redis 故障时服务保持降级可用
func TestReadyDegradesWhenRedisIsUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	health := NewHealth("test", stubChecker{}, stubChecker{err: errors.New("redis down")})
	router := gin.New()
	router.GET("/ready", health.Ready)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ready", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
}

// TestReadyFailsWhenMySQLIsUnavailable 验证 MySQL 故障时就绪检查失败
func TestReadyFailsWhenMySQLIsUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	health := NewHealth("test", stubChecker{err: errors.New("mysql down")}, stubChecker{})
	router := gin.New()
	router.GET("/ready", health.Ready)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ready", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusServiceUnavailable, response.Body.String())
	}
}
