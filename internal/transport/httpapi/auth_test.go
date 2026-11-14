package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/PYang176/Short-URL-Service/internal/apikey"
	"github.com/PYang176/Short-URL-Service/internal/identity"
)

// TestRequireAuth 验证有效 Token 会写入身份上下文和请求 ID
func TestRequireAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := identity.NewTokenManager("0123456789abcdef0123456789abcdef", time.Minute)
	token, _, err := manager.Issue(
		"user-1",
		"workspace-1",
		identity.MemberRoleOwner,
		identity.UserRoleAdmin,
		time.Now().UTC(),
	)
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

// TestRequirePlatformAdmin 验证普通用户不能访问后台，管理员可以访问
func TestRequirePlatformAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := identity.NewTokenManager("0123456789abcdef0123456789abcdef", time.Minute)
	now := time.Now().UTC()
	userToken, _, err := manager.Issue(
		"user-1",
		"workspace-1",
		identity.MemberRoleOwner,
		identity.UserRoleUser,
		now,
	)
	if err != nil {
		t.Fatalf("Issue(user) error = %v", err)
	}
	adminToken, _, err := manager.Issue(
		"admin-1",
		"workspace-admin",
		identity.MemberRoleOwner,
		identity.UserRoleAdmin,
		now,
	)
	if err != nil {
		t.Fatalf("Issue(admin) error = %v", err)
	}

	router := gin.New()
	router.Use(RequestID())
	router.GET(
		"/admin",
		RequireAuth(manager),
		RequirePlatformAdmin(),
		func(c *gin.Context) {
			c.Status(http.StatusNoContent)
		},
	)

	tests := []struct {
		name   string
		token  string
		status int
	}{
		{name: "普通用户", token: userToken, status: http.StatusForbidden},
		{name: "平台管理员", token: adminToken, status: http.StatusNoContent},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/admin", nil)
			request.Header.Set("Authorization", "Bearer "+test.token)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, test.status, response.Body.String())
			}
		})
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

// TestRequireAPIKeyAndScope 验证 API Key 身份上下文和只读作用域
func TestRequireAPIKeyAndScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := identity.NewTokenManager("0123456789abcdef0123456789abcdef", time.Minute)
	authenticator := fakeAPIKeyAuthenticator{
		principal: &apikey.Principal{
			ID:          "key-1",
			WorkspaceID: "workspace-key",
			Scopes:      []string{apikey.ScopeLinksRead},
		},
	}

	router := gin.New()
	router.Use(RequestID())
	router.GET(
		"/read",
		RequireAuth(manager, authenticator),
		RequireScope(apikey.ScopeLinksRead),
		func(c *gin.Context) {
			if got := c.GetString(identity.ContextWorkspaceID); got != "workspace-key" {
				t.Errorf("workspace = %q, want workspace-key", got)
			}
			if got := c.GetString(contextAPIKeyID); got != "key-1" {
				t.Errorf("API Key ID = %q, want key-1", got)
			}
			c.Status(http.StatusNoContent)
		},
	)
	router.GET(
		"/write",
		RequireAuth(manager, authenticator),
		RequireScope(apikey.ScopeLinksWrite),
		func(c *gin.Context) {
			c.Status(http.StatusNoContent)
		},
	)

	readRequest := httptest.NewRequest(http.MethodGet, "/read", nil)
	readRequest.Header.Set("X-API-Key", "sl_live_test")
	readResponse := httptest.NewRecorder()
	router.ServeHTTP(readResponse, readRequest)
	if readResponse.Code != http.StatusNoContent {
		t.Fatalf("read status = %d; body=%s", readResponse.Code, readResponse.Body.String())
	}

	writeRequest := httptest.NewRequest(http.MethodGet, "/write", nil)
	writeRequest.Header.Set("X-API-Key", "sl_live_test")
	writeResponse := httptest.NewRecorder()
	router.ServeHTTP(writeResponse, writeRequest)
	if writeResponse.Code != http.StatusForbidden {
		t.Fatalf("write status = %d, want %d", writeResponse.Code, http.StatusForbidden)
	}
}

type fakeAPIKeyAuthenticator struct {
	principal *apikey.Principal
}

// Authenticate 返回固定的 API Key 测试身份
func (f fakeAPIKeyAuthenticator) Authenticate(
	context.Context,
	string,
) (*apikey.Principal, error) {
	return f.principal, nil
}
