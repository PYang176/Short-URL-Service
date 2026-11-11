package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/PYang176/Short-URL-Service/internal/identity"
)

// TestRequireAuth 验证有效 Token 会写入身份上下文和请求 ID
func TestRequireAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := identity.NewTokenManager("0123456789abcdef0123456789abcdef", time.Minute)
	token, _, err := manager.Issue("user-1", "workspace-1", identity.MemberRoleOwner, time.Now().UTC())
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	router := gin.New()
	router.Use(RequestID())
	router.GET("/protected", RequireAuth(manager), func(c *gin.Context) {
		if got := c.GetString(identity.ContextUserID); got != "user-1" {
			t.Errorf("user ID = %q, want user-1", got)
		}
		if got := c.GetString(identity.ContextWorkspaceID); got != "workspace-1" {
			t.Errorf("workspace ID = %q, want workspace-1", got)
		}
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusNoContent, response.Body.String())
	}
	if response.Header().Get("X-Request-ID") == "" {
		t.Fatal("X-Request-ID is empty")
	}
}

// TestRequireAuthRejectsMissingToken 验证缺少 Authorization 时返回 401
func TestRequireAuthRejectsMissingToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := identity.NewTokenManager("0123456789abcdef0123456789abcdef", time.Minute)

	router := gin.New()
	router.Use(RequestID())
	router.GET("/protected", RequireAuth(manager), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}
