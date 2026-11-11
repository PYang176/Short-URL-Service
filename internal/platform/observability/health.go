package observability

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Checker 定义健康检查使用的依赖探针
type Checker interface {
	// Check 在给定上下文中验证依赖是否可用
	Check(ctx context.Context) error
}

// Health 提供存活与就绪检查 HTTP 处理
type Health struct {
	// version 是响应中展示的构建版本
	version string
	// mysql 是权威数据库探针
	mysql Checker
	// redis 是可降级缓存探针
	redis Checker
	// timeout 是单次依赖探测的最长等待时间
	timeout time.Duration
}

// NewHealth 创建健康检查处理器
func NewHealth(version string, mysql, redis Checker) *Health {
	return &Health{
		version: version,
		mysql:   mysql,
		redis:   redis,
		timeout: 2 * time.Second,
	}
}

// Live 返回进程存活状态，不探测外部依赖
func (h *Health) Live(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"version": h.version,
		"time":    time.Now().UTC(),
	})
}

// Ready 检查服务是否具备接收流量所需的核心依赖
//
// MySQL 不可用时返回 503。Redis 不可用时返回 degraded，因为跳转读路径
// 允许回源 MySQL，服务仍需继续提供基础能力。
func (h *Health) Ready(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.timeout)
	defer cancel()

	mysqlStatus := "ok"
	redisStatus := "ok"
	status := "ok"
	httpStatus := http.StatusOK

	if err := h.mysql.Check(ctx); err != nil {
		mysqlStatus = "unavailable"
		status = "unavailable"
		httpStatus = http.StatusServiceUnavailable
		slog.Error("就绪检查失败", "dependency", "mysql", "error", err)
	}
	if err := h.redis.Check(ctx); err != nil {
		redisStatus = "unavailable"
		if httpStatus == http.StatusOK {
			status = "degraded"
		}
		slog.Warn("就绪检查降级", "dependency", "redis", "error", err)
	}

	c.JSON(httpStatus, gin.H{
		"status": status,
		"checks": gin.H{
			"mysql": mysqlStatus,
			"redis": redisStatus,
		},
		"version": h.version,
		"time":    time.Now().UTC(),
	})
}
