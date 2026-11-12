package link

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/PYang176/Short-URL-Service/internal/identity"
)

// TestHandlerCreateListAndRedirect 验证短链 API 主流程和公开跳转
func TestHandlerCreateListAndRedirect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, _, cleanup := testService(t)
	defer cleanup()
	handler := NewHandler(service, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	router := gin.New()
	api := router.Group("")
	api.Use(withWorkspace("workspace-1"))
	api.POST("/links", handler.Create)
	api.GET("/links", handler.List)
	router.GET("/r/:slug", handler.Redirect)

	createResponse := performLinkRequest(
		t,
		router,
		http.MethodPost,
		"/links",
		`{"destination_url":"https://example.com/handler","alias":"handler","title":"Handler"}`,
	)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("创建状态 = %d, want 201; body=%s", createResponse.Code, createResponse.Body.String())
	}

	var created linkResponse
	if err := json.Unmarshal(createResponse.Body.Bytes(), &created); err != nil {
		t.Fatalf("解析创建响应失败: %v", err)
	}
	if created.ShortURL != "http://localhost:8080/r/handler" {
		t.Fatalf("短链 = %q, want default short URL", created.ShortURL)
	}

	listResponse := performLinkRequest(t, router, http.MethodGet, "/links?limit=20", "")
	if listResponse.Code != http.StatusOK {
		t.Fatalf("列表状态 = %d, want 200; body=%s", listResponse.Code, listResponse.Body.String())
	}

	redirectResponse := performLinkRequest(t, router, http.MethodGet, "/r/handler", "")
	if redirectResponse.Code != http.StatusFound {
		t.Fatalf("跳转状态 = %d, want 302", redirectResponse.Code)
	}
	if location := redirectResponse.Header().Get("Location"); location != "https://example.com/handler" {
		t.Fatalf("Location = %q, want target URL", location)
	}
}

// TestHandlerMapsSlugConflict 验证重复自定义短码返回 409
func TestHandlerMapsSlugConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, _, cleanup := testService(t)
	defer cleanup()
	handler := NewHandler(service, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	router := gin.New()
	router.Use(withWorkspace("workspace-1"))
	router.POST("/links", handler.Create)

	body := `{"destination_url":"https://example.com","alias":"duplicate"}`
	first := performLinkRequest(t, router, http.MethodPost, "/links", body)
	if first.Code != http.StatusCreated {
		t.Fatalf("第一次创建状态 = %d, want 201", first.Code)
	}
	second := performLinkRequest(t, router, http.MethodPost, "/links", body)
	if second.Code != http.StatusConflict {
		t.Fatalf("重复短码状态 = %d, want 409; body=%s", second.Code, second.Body.String())
	}
}

// TestCursorRoundTrip 验证分页游标编码和解析
func TestCursorRoundTrip(t *testing.T) {
	service, _, cleanup := testService(t)
	defer cleanup()
	created, err := service.Create(t.Context(), "workspace-1", CreateInput{
		DestinationURL: "https://example.com/cursor",
		Alias:          "cursor",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	createdAt, id, err := decodeCursor(encodeCursor(*created))
	if err != nil {
		t.Fatalf("decodeCursor() error = %v", err)
	}
	if id != created.ID || createdAt == nil || !createdAt.Equal(created.CreatedAt) {
		t.Fatalf("游标往返结果不匹配")
	}
}

// withWorkspace 为测试路由注入固定工作空间
func withWorkspace(workspaceID string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(identity.ContextWorkspaceID, workspaceID)
		c.Next()
	}
}

// performLinkRequest 构造并执行短链测试请求
func performLinkRequest(
	t *testing.T,
	handler http.Handler,
	method, path, body string,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
