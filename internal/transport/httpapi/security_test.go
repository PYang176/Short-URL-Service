package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestJSONContentTypeRejectsNonJSONBody 验证有请求体时必须使用 JSON
func TestJSONContentTypeRejectsNonJSONBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID(), JSONContentType())
	router.POST("/test", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader("not-json"))
	request.Header.Set("Content-Type", "text/plain")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnsupportedMediaType)
	}
}

// TestJSONContentTypeAllowsBulkMultipart 验证 CSV 批量接口允许 multipart 上传
func TestJSONContentTypeAllowsBulkMultipart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID(), JSONContentType())
	router.POST("/api/v1/links/bulk", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/links/bulk",
		strings.NewReader("multipart-body"),
	)
	request.Header.Set("Content-Type", "multipart/form-data; boundary=test")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
}

// TestBodyLimitRejectsLargeRequest 验证超过上限的请求体返回 413
func TestBodyLimitRejectsLargeRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID(), BodyLimit(4))
	router.POST("/test", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader("large"))
	request.ContentLength = 5
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusRequestEntityTooLarge)
	}
}

// TestBodyLimitRulesAllowsBulkOverride 验证批量路径可以使用更高请求体上限
func TestBodyLimitRulesAllowsBulkOverride(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID(), BodyLimitRules(4, map[string]int64{
		"/api/v1/links/bulk": 16,
	}))
	router.POST("/api/v1/links/bulk", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/links/bulk",
		strings.NewReader("larger-body"),
	)
	request.ContentLength = int64(len("larger-body"))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
}
