package httpapi

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// SecurityHeaders 添加 API 通用安全响应头
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		c.Next()
	}
}

// BodyLimit 限制 API 请求体最大字节数
func BodyLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.ContentLength > maxBytes {
			requestID, _ := c.Get(requestIDKey)
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{
				"error": gin.H{
					"code":       "PAYLOAD_TOO_LARGE",
					"message":    "请求体过大",
					"request_id": requestID,
				},
			})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}

// JSONContentType 拒绝修改类 API 的非 JSON 请求
func JSONContentType() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodPost ||
			c.Request.Method == http.MethodPut ||
			c.Request.Method == http.MethodPatch {
			if c.Request.ContentLength == 0 {
				c.Next()
				return
			}
			contentType := strings.ToLower(c.GetHeader("Content-Type"))
			if !strings.HasPrefix(contentType, "application/json") {
				requestID, _ := c.Get(requestIDKey)
				c.AbortWithStatusJSON(http.StatusUnsupportedMediaType, gin.H{
					"error": gin.H{
						"code":       "UNSUPPORTED_MEDIA_TYPE",
						"message":    "Content-Type 必须为 application/json",
						"request_id": requestID,
					},
				})
				return
			}
		}
		c.Next()
	}
}
