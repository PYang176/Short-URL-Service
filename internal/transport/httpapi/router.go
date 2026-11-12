package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/PYang176/Short-URL-Service/internal/analytics"
	"github.com/PYang176/Short-URL-Service/internal/identity"
	"github.com/PYang176/Short-URL-Service/internal/link"
	"github.com/PYang176/Short-URL-Service/internal/platform/config"
	"github.com/PYang176/Short-URL-Service/internal/platform/observability"
)

// Dependencies 汇总构建 HTTP 路由所需的配置和处理器
type Dependencies struct {
	// Config 是 API 运行时配置
	Config config.Config
	// Logger 是结构化访问和错误日志
	Logger *slog.Logger
	// Registry 是本进程独立的 Prometheus 注册表
	Registry *prometheus.Registry
	// Health 是存活和就绪处理器
	Health *observability.Health
	// Metrics 是 HTTP 指标中间件
	Metrics *observability.Metrics
	// Identity 是认证和资料接口处理器
	Identity *identity.Handler
	// Links 是短链管理和公开跳转接口处理器
	Links *link.Handler
	// Analytics 是点击分析接口处理器
	Analytics *analytics.Handler
	// Tokens 是受保护路由使用的 Access Token 管理器
	Tokens *identity.TokenManager
	// APIVersion 是响应中展示的接口版本
	APIVersion string
}

// NewRouter 组装中间件、公开路由和受保护 API 路由
func NewRouter(deps Dependencies) *gin.Engine {
	switch deps.Config.Env {
	case config.EnvProduction:
		gin.SetMode(gin.ReleaseMode)
	case config.EnvTest:
		gin.SetMode(gin.TestMode)
	default:
		gin.SetMode(gin.DebugMode)
	}

	router := gin.New()
	router.Use(
		RequestID(),
		AccessLog(deps.Logger),
		deps.Metrics.Middleware(),
		Recovery(deps.Logger),
	)

	router.GET("/health/live", deps.Health.Live)
	router.GET("/health/ready", deps.Health.Ready)
	router.GET("/metrics", gin.WrapH(promhttp.HandlerFor(deps.Registry, promhttp.HandlerOpts{})))

	api := router.Group("/api/v1")
	api.GET("/health", deps.Health.Live)
	api.GET("/version", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"version": deps.Config.Version,
			"api":     deps.APIVersion,
		})
	})

	auth := api.Group("/auth")
	auth.POST("/register", deps.Identity.Register)
	auth.POST("/login", deps.Identity.Login)
	auth.POST("/refresh", deps.Identity.Refresh)
	auth.POST("/logout", deps.Identity.Logout)

	protected := api.Group("")
	protected.Use(RequireAuth(deps.Tokens))
	protected.GET("/me", deps.Identity.Profile)
	protected.PATCH("/me", deps.Identity.UpdateProfile)
	protected.POST("/links", deps.Links.Create)
	protected.GET("/links", deps.Links.List)
	protected.GET("/links/:id", deps.Links.Get)
	protected.PATCH("/links/:id", deps.Links.Update)
	protected.DELETE("/links/:id", deps.Links.Delete)
	protected.GET("/links/:id/analytics", deps.Analytics.Overview)

	router.GET("/r/:slug", deps.Links.Redirect)

	router.NoRoute(func(c *gin.Context) {
		requestID, _ := c.Get(requestIDKey)
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"code":       "NOT_FOUND",
				"message":    "接口不存在",
				"request_id": requestID,
			},
		})
	})

	return router
}
