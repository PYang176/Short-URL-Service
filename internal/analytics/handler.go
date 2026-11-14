package analytics

import (
	"encoding/csv"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

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

// WorkspaceOverview 返回当前工作空间最近 days 天的整体分析
func (h *Handler) WorkspaceOverview(c *gin.Context) {
	days := 7
	if raw := c.Query("days"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			writeError(c, http.StatusBadRequest, "INVALID_DAYS", "days 必须为数字")
			return
		}
		days = parsed
	}

	overview, err := h.service.WorkspaceOverview(
		c.Request.Context(),
		c.GetString(identity.ContextWorkspaceID),
		days,
	)
	if err != nil {
		if errors.Is(err, link.ErrInvalidInput) {
			writeError(c, http.StatusBadRequest, "INVALID_DAYS", "days 必须为 1 至 90")
			return
		}
		h.logger.Error("查询工作空间分析失败", "error", err)
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "服务器内部错误")
		return
	}
	c.JSON(http.StatusOK, overview)
}

// ExportCSV 流式导出当前工作空间或单链接的点击明细
func (h *Handler) ExportCSV(c *gin.Context) {
	days := 7
	if raw := c.Query("days"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			writeError(c, http.StatusBadRequest, "INVALID_DAYS", "days 必须为数字")
			return
		}
		days = parsed
	}
	reader, err := h.service.OpenExport(
		c.Request.Context(),
		c.GetString(identity.ContextWorkspaceID),
		ExportInput{
			LinkID:    c.Query("link_id"),
			Days:      days,
			Visitor:   c.Query("visitor"),
			Device:    c.Query("device"),
			Country:   c.Query("country"),
			Browser:   c.Query("browser"),
			Referrer:  c.Query("referrer"),
			UTMSource: c.Query("utm_source"),
		},
	)
	if err != nil {
		switch {
		case errors.Is(err, link.ErrInvalidInput):
			writeError(c, http.StatusBadRequest, "INVALID_EXPORT_FILTER", err.Error())
		case errors.Is(err, link.ErrNotFound):
			writeError(c, http.StatusNotFound, "LINK_NOT_FOUND", "短链不存在")
		default:
			h.logger.Error("打开点击明细导出失败", "error", err)
			writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "服务器内部错误")
		}
		return
	}
	defer func() {
		if err := reader.Close(); err != nil {
			h.logger.Warn("关闭点击明细导出游标失败", "error", err)
		}
	}()

	filename := "shorturl-clicks-" + time.Now().UTC().Format("20060102-150405") + ".csv"
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	_, _ = c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(c.Writer)
	if err := writer.Write([]string{
		"event_id",
		"clicked_at",
		"slug",
		"country",
		"device",
		"os",
		"browser",
		"language",
		"referrer",
		"utm_source",
		"utm_medium",
		"utm_campaign",
		"utm_term",
		"utm_content",
		"is_bot",
	}); err != nil {
		h.logger.Warn("写入 CSV 表头失败", "error", err)
		return
	}

	for reader.Next() {
		event := reader.Event()
		if err := writer.Write([]string{
			event.EventID,
			event.ClickedAt.UTC().Format(time.RFC3339Nano),
			safeCSVCell(event.Slug),
			safeCSVCell(event.Country),
			safeCSVCell(event.Device),
			safeCSVCell(event.OS),
			safeCSVCell(event.Browser),
			safeCSVCell(event.Language),
			safeCSVCell(event.Referrer),
			safeCSVCell(event.UTMSource),
			safeCSVCell(event.UTMMedium),
			safeCSVCell(event.UTMCampaign),
			safeCSVCell(event.UTMTerm),
			safeCSVCell(event.UTMContent),
			strconv.FormatBool(event.IsBot),
		}); err != nil {
			h.logger.Warn("写入点击明细 CSV 失败", "error", err)
			return
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		h.logger.Warn("刷新点击明细 CSV 失败", "error", err)
	}
	if err := reader.Err(); err != nil {
		h.logger.Warn("读取点击明细导出游标失败", "error", err)
	}
}

// safeCSVCell 阻止表格软件把用户字段解释为公式
func safeCSVCell(value string) string {
	if value == "" {
		return value
	}
	switch value[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + value
	default:
		return value
	}
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
