package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"

	"github.com/PYang176/Short-URL-Service/internal/abuse"
	"github.com/PYang176/Short-URL-Service/internal/analytics"
	"github.com/PYang176/Short-URL-Service/internal/apikey"
	"github.com/PYang176/Short-URL-Service/internal/domain"
	"github.com/PYang176/Short-URL-Service/internal/identity"
	"github.com/PYang176/Short-URL-Service/internal/link"
	"github.com/PYang176/Short-URL-Service/internal/platform/config"
	"github.com/PYang176/Short-URL-Service/internal/platform/observability"
	"github.com/PYang176/Short-URL-Service/internal/platform/ratelimit"
	"github.com/PYang176/Short-URL-Service/internal/trial"
	"github.com/PYang176/Short-URL-Service/internal/webhook"
)

// Dependencies 汇总构建 HTTP 路由所需的配置和处理器
type Dependencies struct {
	// Config 是 API 运行时配置
	Config config.Config
	// Logger 是结构化访问和错误日志
	Logger *slog.Logger
	// Health 是存活和就绪处理器
	Health *observability.Health
	// Metrics 是 HTTP 指标中间件
	Metrics *observability.Metrics
	// Identity 是认证和资料接口处理器
	Identity *identity.Handler
	// Links 是短链管理和公开跳转接口处理器
	Links *link.Handler
	// Domains 是自定义域名接口处理器
	Domains *domain.Handler
	// Analytics 是点击分析接口处理器
	Analytics *analytics.Handler
	// APIKeys 是 API Key 生命周期接口处理器
	APIKeys *apikey.Handler
	// APIKeyAuth 是资源接口使用的 API Key 认证器
	APIKeyAuth APIKeyAuthenticator
	// Trial 是匿名游客试用接口处理器
	Trial *trial.Handler
	// Webhooks 是 Webhook 配置和投递记录接口处理器
	Webhooks *webhook.Handler
	// Abuse 是公开举报和管理员治理接口处理器
	Abuse *abuse.Handler
	// Tokens 是受保护路由使用的 Access Token 管理器
	Tokens *identity.TokenManager
	// Limiter 是共享 Redis 滑动窗口限流器
	Limiter *ratelimit.Limiter
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
	if err := router.SetTrustedProxies(deps.Config.Security.TrustedProxies); err != nil {
		deps.Logger.Error("配置可信代理失败，禁用代理头解析", "error", err)
		_ = router.SetTrustedProxies(nil)
	}
	binding.EnableDecoderDisallowUnknownFields = true
	binding.EnableDecoderUseNumber = true
	router.Use(
		SecurityHeaders(),
		RequestID(),
		AccessLog(deps.Logger),
		deps.Metrics.Middleware(),
		Recovery(deps.Logger),
	)

	router.GET("/health/live", deps.Health.Live)
	router.GET("/health/ready", deps.Health.Ready)
	api := router.Group("/api/v1")
	api.Use(BodyLimit(deps.Config.Security.MaxBodyBytes), JSONContentType())
	api.GET("/health", deps.Health.Live)
	api.GET("/version", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"version": deps.Config.Version,
			"api":     deps.APIVersion,
		})
	})

	auth := api.Group("/auth")
	if deps.Config.RateLimit.Enabled {
		auth.POST("/register",
			RateLimitByIP(deps.Limiter, deps.Logger, "auth:register", deps.Config.RateLimit.Register),
			deps.Identity.Register,
		)
		auth.POST("/login",
			RateLimitByIP(deps.Limiter, deps.Logger, "auth:login", deps.Config.RateLimit.Login),
			deps.Identity.Login,
		)
		auth.POST("/refresh",
			RateLimitByIP(deps.Limiter, deps.Logger, "auth:refresh", deps.Config.RateLimit.Refresh),
			deps.Identity.Refresh,
		)
	} else {
		auth.POST("/register", deps.Identity.Register)
		auth.POST("/login", deps.Identity.Login)
		auth.POST("/refresh", deps.Identity.Refresh)
	}
	auth.POST("/logout", deps.Identity.Logout)

	trialAPI := api.Group("/trial")
	if deps.Config.RateLimit.Enabled {
		trialAPI.Use(RateLimitByIP(
			deps.Limiter,
			deps.Logger,
			"trial:default",
			deps.Config.RateLimit.API,
		))
		trialAPI.POST(
			"/links",
			RateLimitByIP(
				deps.Limiter,
				deps.Logger,
				"trial:create",
				deps.Config.RateLimit.TrialCreate,
			),
			deps.Trial.Create,
		)
	} else {
		trialAPI.POST("/links", deps.Trial.Create)
	}
	trialAPI.GET("/links", deps.Trial.List)

	abuseAPI := api.Group("/abuse")
	if deps.Config.RateLimit.Enabled {
		abuseAPI.POST(
			"/reports",
			RateLimitByIP(
				deps.Limiter,
				deps.Logger,
				"abuse:report",
				deps.Config.RateLimit.AbuseReport,
			),
			deps.Abuse.CreateReport,
		)
	} else {
		abuseAPI.POST("/reports", deps.Abuse.CreateReport)
	}

	protected := api.Group("")
	protected.Use(RequireAuth(deps.Tokens))
	if deps.Config.RateLimit.Enabled {
		protected.Use(RateLimitByWorkspace(deps.Limiter, deps.Logger, "api:default", deps.Config.RateLimit.API))
	}
	protected.GET("/me", deps.Identity.Profile)
	protected.PATCH("/me", deps.Identity.UpdateProfile)
	protected.POST("/trial/claim", deps.Trial.Claim)

	admin := protected.Group("")
	admin.Use(RequirePlatformAdmin())
	admin.GET("/api-keys", deps.APIKeys.List)
	admin.POST("/api-keys", deps.APIKeys.Create)
	admin.DELETE("/api-keys/:id", deps.APIKeys.Revoke)
	admin.GET("/webhooks", deps.Webhooks.List)
	admin.POST("/webhooks", deps.Webhooks.Create)
	admin.DELETE("/webhooks/:id", deps.Webhooks.Disable)
	admin.GET("/webhooks/:id/deliveries", deps.Webhooks.ListDeliveries)
	admin.GET("/abuse/reports", deps.Abuse.ListReports)
	admin.POST("/abuse/reports/:id/resolve", deps.Abuse.ResolveReport)
	admin.GET("/abuse/blocked-domains", deps.Abuse.ListBlockedDomains)
	admin.POST("/abuse/blocked-domains", deps.Abuse.AddBlockedDomain)
	admin.DELETE("/abuse/blocked-domains/:id", deps.Abuse.RemoveBlockedDomain)
	admin.GET("/domains", deps.Domains.List)
	admin.POST("/domains", deps.Domains.Add)
	admin.POST("/domains/:id/verify", deps.Domains.Verify)
	admin.DELETE("/domains/:id", deps.Domains.Delete)

	resources := api.Group("")
	resources.Use(RequireAuth(deps.Tokens, deps.APIKeyAuth))
	if deps.Config.RateLimit.Enabled {
		resources.Use(RateLimitByPrincipal(
			deps.Limiter,
			deps.Logger,
			"api:resource",
			deps.Config.RateLimit.API,
		))
		resources.POST("/links",
			RequireScope(apikey.ScopeLinksWrite),
			RateLimitByPrincipal(deps.Limiter, deps.Logger, "api:create-link", deps.Config.RateLimit.CreateLink),
			deps.Links.Create,
		)
	} else {
		resources.POST("/links", RequireScope(apikey.ScopeLinksWrite), deps.Links.Create)
	}
	resources.GET("/links", RequireScope(apikey.ScopeLinksRead), deps.Links.List)
	resources.GET("/links/:id", RequireScope(apikey.ScopeLinksRead), deps.Links.Get)
	resources.PATCH("/links/:id", RequireScope(apikey.ScopeLinksWrite), deps.Links.Update)
	resources.DELETE("/links/:id", RequireScope(apikey.ScopeLinksWrite), deps.Links.Delete)
	resources.GET("/links/:id/qr", RequireScope(apikey.ScopeLinksRead), deps.Links.QRCode)
	resources.GET("/links/:id/analytics", RequireScope(apikey.ScopeAnalyticsRead), deps.Analytics.Overview)
	resources.GET("/analytics/overview", RequireScope(apikey.ScopeAnalyticsRead), deps.Analytics.WorkspaceOverview)

	if deps.Config.RateLimit.Enabled {
		redirectLimit := RateLimitByIP(deps.Limiter, deps.Logger, "redirect", deps.Config.RateLimit.Redirect)
		router.GET("/r/:slug", redirectLimit, deps.Links.Redirect)
		router.GET("/:slug", redirectLimit, deps.Links.RedirectByHost)
	} else {
		router.GET("/r/:slug", deps.Links.Redirect)
		router.GET("/:slug", deps.Links.RedirectByHost)
	}

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
