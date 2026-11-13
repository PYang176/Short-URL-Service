package httpapi

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/PYang176/Short-URL-Service/internal/identity"
	"github.com/PYang176/Short-URL-Service/internal/platform/config"
	"github.com/PYang176/Short-URL-Service/internal/platform/ratelimit"
)

// RateLimitByIP 以客户端 IP 为维度应用 Redis 限流
func RateLimitByIP(
	limiter *ratelimit.Limiter,
	logger *slog.Logger,
	scope string,
	rule config.RateLimitRule,
) gin.HandlerFunc {
	return rateLimit(limiter, logger, scope, rule, func(c *gin.Context) string {
		return c.ClientIP()
	})
}

// RateLimitByWorkspace 以当前工作空间为维度应用 Redis 限流
func RateLimitByWorkspace(
	limiter *ratelimit.Limiter,
	logger *slog.Logger,
	scope string,
	rule config.RateLimitRule,
) gin.HandlerFunc {
	return rateLimit(limiter, logger, scope, rule, func(c *gin.Context) string {
		if workspaceID := c.GetString(identity.ContextWorkspaceID); workspaceID != "" {
			return "workspace:" + workspaceID
		}
		return "ip:" + c.ClientIP()
	})
}

// RateLimitByPrincipal 按 API Key+IP 或工作空间应用资源接口限流
func RateLimitByPrincipal(
	limiter *ratelimit.Limiter,
	logger *slog.Logger,
	scope string,
	rule config.RateLimitRule,
) gin.HandlerFunc {
	return rateLimit(limiter, logger, scope, rule, func(c *gin.Context) string {
		if apiKeyID := c.GetString(contextAPIKeyID); apiKeyID != "" {
			return "api-key:" + apiKeyID + ":ip:" + c.ClientIP()
		}
		if workspaceID := c.GetString(identity.ContextWorkspaceID); workspaceID != "" {
			return "workspace:" + workspaceID
		}
		return "ip:" + c.ClientIP()
	})
}

// rateLimit 构建统一的拒绝响应和故障策略
func rateLimit(
	limiter *ratelimit.Limiter,
	logger *slog.Logger,
	scope string,
	rule config.RateLimitRule,
	identityKey func(*gin.Context) string,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID, _ := c.Get(requestIDKey)
		member := uuid.NewString()
		if value, ok := requestID.(string); ok && value != "" {
			member = value
		}
		key := "ratelimit:v1:" + scope + ":" + identityKey(c)
		decision, err := limiter.Allow(c.Request.Context(), key, member, rule)
		if err != nil {
			logger.Warn("Redis 限流不可用",
				"scope", scope,
				"fail_closed", rule.FailClosed,
				"error", err,
			)
			if rule.FailClosed {
				writeRateLimitError(c, http.StatusServiceUnavailable, "RATE_LIMITER_UNAVAILABLE", "限流服务暂时不可用")
				return
			}
			c.Next()
			return
		}

		c.Header("X-RateLimit-Limit", strconv.Itoa(rule.Limit))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(max(0, rule.Limit-decision.Count)))
		if !decision.Allowed {
			retrySeconds := max(1, int(decision.RetryAfter.Seconds()))
			c.Header("Retry-After", strconv.Itoa(retrySeconds))
			writeRateLimitError(c, http.StatusTooManyRequests, "RATE_LIMITED", "请求过于频繁，请稍后重试")
			return
		}
		c.Next()
	}
}

// writeRateLimitError 输出统一限流错误
func writeRateLimitError(c *gin.Context, status int, code, message string) {
	requestID, _ := c.Get(requestIDKey)
	c.AbortWithStatusJSON(status, gin.H{
		"error": gin.H{
			"code":       code,
			"message":    message,
			"request_id": requestID,
		},
	})
}
