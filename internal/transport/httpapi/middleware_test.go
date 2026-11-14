package httpapi

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestAccessLogOmitsClientIP 验证访问日志不写入客户端原始 IP。
func TestAccessLogOmitsClientIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	router := gin.New()
	router.Use(RequestID(), AccessLog(logger))
	router.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/test", nil)
	request.RemoteAddr = "198.51.100.77:4321"
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", response.Code)
	}
	if strings.Contains(output.String(), "198.51.100.77") {
		t.Fatalf("访问日志包含原始 IP: %s", output.String())
	}
}
