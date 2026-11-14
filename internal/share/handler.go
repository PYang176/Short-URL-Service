package share

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/PYang176/Short-URL-Service/internal/analytics"
	"github.com/PYang176/Short-URL-Service/internal/identity"
	"github.com/PYang176/Short-URL-Service/internal/link"
)

// Handler 提供所有者分享管理和公开只读报告接口
type Handler struct {
	// service 管理分享生命周期
	service *Service
	// links 校验短链归属并生成公开短链地址
	links *link.Service
	// analytics 提供脱敏分析聚合
	analytics *analytics.Service
	// logger 记录内部错误
	logger *slog.Logger
}

// NewHandler 创建公开分析分享处理器
func NewHandler(
	service *Service,
	links *link.Service,
	analyticsService *analytics.Service,
	logger *slog.Logger,
) *Handler {
	return &Handler{
		service:   service,
		links:     links,
		analytics: analyticsService,
		logger:    logger,
	}
}

type createRequest struct {
	// Days 是公开报告固定的分析范围
	Days int `json:"days" binding:"required"`
}

type shareResponse struct {
	// ID 是 ULID 分享标识
	ID string `json:"id"`
	// Days 是分析范围
	Days int `json:"days"`
	// ExpiresAt 是分享失效时间
	ExpiresAt time.Time `json:"expires_at"`
	// CreatedAt 是创建时间
	CreatedAt time.Time `json:"created_at"`
}

type createResponse struct {
	// Share 是新创建的分享元数据
	Share shareResponse `json:"share"`
	// Token 是只在创建时返回的公开分享 Token
	Token string `json:"token"`
}

type publicAnalyticsResponse struct {
	// Summary 是核心访问指标
	Summary analytics.Summary `json:"summary"`
	// Trend 是访问趋势
	Trend []analytics.TrendPoint `json:"trend"`
	// Countries 是国家或地区分布
	Countries []analytics.BreakdownItem `json:"countries"`
	// Devices 是设备分布
	Devices []analytics.BreakdownItem `json:"devices"`
	// OperatingSystems 是操作系统分布
	OperatingSystems []analytics.BreakdownItem `json:"operating_systems"`
	// Browsers 是浏览器分布
	Browsers []analytics.BreakdownItem `json:"browsers"`
}

type publicResponse struct {
	PublicReport
	// Analytics 是公开可展示的聚合分析
	Analytics publicAnalyticsResponse `json:"analytics"`
}

// Create 为当前用户工作空间内的短链创建公开分享
func (h *Handler) Create(c *gin.Context) {
	workspaceID := c.GetString(identity.ContextWorkspaceID)
	userID := c.GetString(identity.ContextUserID)
	linkRecord, err := h.links.Get(c.Request.Context(), workspaceID, c.Param("id"))
	if err != nil {
		h.writeError(c, http.StatusNotFound, "LINK_NOT_FOUND", "短链不存在")
		return
	}
	var request createRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, http.StatusBadRequest, "INVALID_SHARE", "分享范围必须为 1、7 或 30 天")
		return
	}
	result, err := h.service.Create(
		c.Request.Context(),
		workspaceID,
		userID,
		linkRecord.ID,
		request.Days,
	)
	if err != nil {
		h.writeShareError(c, err)
		return
	}
	c.JSON(http.StatusCreated, createResponse{
		Share: toResponse(result.Share),
		Token: result.Token,
	})
}

// List 返回短链当前有效的公开分享
func (h *Handler) List(c *gin.Context) {
	workspaceID := c.GetString(identity.ContextWorkspaceID)
	linkRecord, err := h.links.Get(c.Request.Context(), workspaceID, c.Param("id"))
	if err != nil {
		h.writeError(c, http.StatusNotFound, "LINK_NOT_FOUND", "短链不存在")
		return
	}
	items, err := h.service.List(c.Request.Context(), workspaceID, linkRecord.ID)
	if err != nil {
		h.logger.Error("查询公开分享列表失败", "error", err)
		h.writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "服务器内部错误")
		return
	}
	responses := make([]shareResponse, 0, len(items))
	for index := range items {
		responses = append(responses, toResponse(items[index]))
	}
	c.JSON(http.StatusOK, gin.H{"items": responses})
}

// Revoke 撤销短链的指定公开分享
func (h *Handler) Revoke(c *gin.Context) {
	workspaceID := c.GetString(identity.ContextWorkspaceID)
	linkRecord, err := h.links.Get(c.Request.Context(), workspaceID, c.Param("id"))
	if err != nil {
		h.writeError(c, http.StatusNotFound, "LINK_NOT_FOUND", "短链不存在")
		return
	}
	if err := h.service.Revoke(
		c.Request.Context(),
		workspaceID,
		linkRecord.ID,
		c.Param("shareId"),
	); err != nil {
		h.writeShareError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// Public 返回公开 Token 对应的脱敏只读分析报告
func (h *Handler) Public(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	item, err := h.service.Resolve(c.Request.Context(), c.Param("token"))
	if err != nil {
		h.writeShareError(c, err)
		return
	}
	linkRecord, err := h.links.Get(c.Request.Context(), item.WorkspaceID, item.LinkID)
	if err != nil {
		h.writeError(c, http.StatusNotFound, "SHARE_NOT_FOUND", "分享不存在或已失效")
		return
	}
	overview, err := h.analytics.Overview(
		c.Request.Context(),
		item.WorkspaceID,
		item.LinkID,
		item.Days,
	)
	if err != nil {
		h.logger.Error("查询公开分析分享失败", "share_id", item.ID, "error", err)
		h.writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "服务器内部错误")
		return
	}
	title := strings.TrimSpace(linkRecord.Title)
	if title == "" {
		title = "/" + linkRecord.Slug
	}
	c.JSON(http.StatusOK, publicResponse{
		PublicReport: PublicReport{
			Title:     title,
			Slug:      linkRecord.Slug,
			ShortURL:  h.links.ShortURL(linkRecord),
			Days:      item.Days,
			ExpiresAt: item.ExpiresAt,
		},
		Analytics: publicAnalyticsResponse{
			Summary:          overview.Summary,
			Trend:            overview.Trend,
			Countries:        overview.Countries,
			Devices:          overview.Devices,
			OperatingSystems: overview.OperatingSystems,
			Browsers:         overview.Browsers,
		},
	})
}

// toResponse 将分享模型转换为所有者管理 DTO
func toResponse(item Share) shareResponse {
	return shareResponse{
		ID:        item.ID,
		Days:      item.Days,
		ExpiresAt: item.ExpiresAt,
		CreatedAt: item.CreatedAt,
	}
}

// writeShareError 将分享领域错误映射为 HTTP 响应
func (h *Handler) writeShareError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		h.writeError(c, http.StatusBadRequest, "INVALID_SHARE", err.Error())
	case errors.Is(err, ErrNotFound):
		h.writeError(c, http.StatusNotFound, "SHARE_NOT_FOUND", "分享不存在或已失效")
	default:
		h.logger.Error("处理公开分析分享失败", "error", err)
		h.writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "服务器内部错误")
	}
}

// writeError 输出带请求 ID 的统一错误响应
func (h *Handler) writeError(c *gin.Context, status int, code, message string) {
	requestID, _ := c.Get("request_id")
	c.JSON(status, gin.H{
		"error": gin.H{
			"code":       code,
			"message":    message,
			"request_id": requestID,
		},
	})
}
