package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/PYang176/Short-URL-Service/internal/apikey"
	"github.com/PYang176/Short-URL-Service/internal/identity"
)

const (
	// contextAPIKeyID 是 Gin 上下文中的 API Key 记录 ID 键
	contextAPIKeyID = "api_key_id"
	// contextAPIKeyScopes 是 Gin 上下文中的 API Key 作用域键
	contextAPIKeyScopes = "api_key_scopes"
)

// APIKeyAuthenticator 定义 HTTP 层需要的 API Key 认证能力
type APIKeyAuthenticator interface {
	// Authenticate 校验原始 API Key 并返回最小身份上下文
	Authenticate(ctx context.Context, raw string) (*apikey.Principal, error)
}

// RequireAuth 校验 Access Token 并写入当前用户、工作空间和角色
func RequireAuth(
	tokens *identity.TokenManager,
	apiKeys ...APIKeyAuthenticator,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		authorization := strings.TrimSpace(c.GetHeader("Authorization"))
		apiKeyHeader := strings.TrimSpace(c.GetHeader("X-API-Key"))
		rawAPIKey := apiKeyHeader
		parts := strings.SplitN(authorization, " ", 2)
		if rawAPIKey == "" && len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			rawAPIKey = strings.TrimSpace(parts[1])
		}
		if strings.HasPrefix(rawAPIKey, "sl_live_") {
			if len(apiKeys) == 0 || apiKeys[0] == nil {
				abortUnauthorized(c)
				return
			}
			principal, err := apiKeys[0].Authenticate(c.Request.Context(), rawAPIKey)
			if err != nil {
				abortUnauthorized(c)
				return
			}
			c.Set(identity.ContextWorkspaceID, principal.WorkspaceID)
			c.Set(identity.ContextMemberRole, identity.MemberRoleMember)
			c.Set(contextAPIKeyID, principal.ID)
			c.Set(contextAPIKeyScopes, principal.Scopes)
			c.Next()
			return
		}
		if rawAPIKey != "" && apiKeyHeader != "" {
			abortUnauthorized(c)
			return
		}
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
			abortUnauthorized(c)
			return
		}

		claims, err := tokens.Parse(strings.TrimSpace(parts[1]), time.Now().UTC())
		if err != nil {
			abortUnauthorized(c)
			return
		}
		c.Set(identity.ContextUserID, claims.Subject)
		c.Set(identity.ContextWorkspaceID, claims.WorkspaceID)
		c.Set(identity.ContextMemberRole, claims.Role)
		c.Next()
	}
}

// RequireScope 校验 API Key 是否包含指定作用域；JWT 请求默认拥有全部作用域
func RequireScope(scope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString(contextAPIKeyID) == "" {
			c.Next()
			return
		}
		value, ok := c.Get(contextAPIKeyScopes)
		scopes, valid := value.([]string)
		if !ok || !valid {
			abortForbidden(c, "API Key 缺少作用域")
			return
		}
		for _, granted := range scopes {
			if granted == scope {
				c.Next()
				return
			}
		}
		abortForbidden(c, "API Key 缺少 "+scope+" 作用域")
	}
}

// abortUnauthorized 输出统一的未认证错误并终止后续处理
func abortUnauthorized(c *gin.Context) {
	requestID, _ := c.Get(requestIDKey)
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"error": gin.H{
			"code":       "UNAUTHORIZED",
			"message":    "需要登录后访问",
			"request_id": requestID,
		},
	})
}

// abortForbidden 输出作用域不足错误并终止后续处理
func abortForbidden(c *gin.Context, message string) {
	requestID, _ := c.Get(requestIDKey)
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
		"error": gin.H{
			"code":       "FORBIDDEN",
			"message":    message,
			"request_id": requestID,
		},
	})
}
