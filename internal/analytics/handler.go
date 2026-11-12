package analytics

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/PYang176/Short-URL-Service/internal/identity"
	"github.com/PYang176/Short-URL-Service/internal/link"
)

// Handler 提供链接点击分析 HTTP 接口
type Handler struct {
	// service 提供带链接归属校验的分析查询
	service *Service
	// logger 记录未映射的内部错误
	logger *slog.Logger
}

// NewHandler 创建点击分析 HTTP 处理器
func NewHandler(service *Service, logger *slog.Logger) *Handler {
	return &Handler{service: service, logger: logger}
}

// Overview 返回当前工作空间内单个链接的分析结果
func (h *Handler) Overview(c *gin.Context) {
	days := 30
	if raw := c.Query("days"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			writeError(c, http.StatusBadRequest, "INVALID_DAYS", "days 必须为数字")
			return
		}
		days = parsed
	}

	overview, err := h.service.Overview(
		c.Request.Context(),
		c.GetString(identity.ContextWorkspaceID),
		c.Param("id"),
		days,
	)
	if err != nil {
		switch {
		case errors.Is(err, link.ErrInvalidInput):
			writeError(c, http.StatusBadRequest, "INVALID_DAYS", "days 必须为 1 至 90")
		case errors.Is(err, link.ErrNotFound):
			writeError(c, http.StatusNotFound, "LINK_NOT_FOUND", "短链不存在")
		default:
			h.logger.Error("查询链接分析失败", "link_id", c.Param("id"), "error", err)
			writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "服务器内部错误")
		}
		return
	}
	c.JSON(http.StatusOK, overview)
}

// writeError 输出包含请求 ID 的统一错误响应
func writeError(c *gin.Context, status int, code, message string) {
	requestID, _ := c.Get("request_id")
	c.JSON(status, gin.H{
		"error": gin.H{
			"code":       code,
			"message":    message,
			"request_id": requestID,
		},
	})
}
