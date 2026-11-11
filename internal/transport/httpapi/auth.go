package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/PYang176/Short-URL-Service/internal/identity"
)

// RequireAuth 校验 Access Token 并写入当前用户、工作空间和角色
func RequireAuth(tokens *identity.TokenManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		authorization := strings.TrimSpace(c.GetHeader("Authorization"))
		parts := strings.SplitN(authorization, " ", 2)
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
