package abuse

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/PYang176/Short-URL-Service/internal/identity"
)

// Handler 提供公开举报与管理员治理 HTTP 接口
type Handler struct {
	// service 管理举报、下架和域名黑名单
	service *Service
	// logger 记录内部错误
	logger *slog.Logger
}

// NewHandler 创建滥用治理 HTTP 处理器
func NewHandler(service *Service, logger *slog.Logger) *Handler {
	return &Handler{service: service, logger: logger}
}

type createReportRequest struct {
	// ShortURL 是用户看到的完整短链地址
	ShortURL string `json:"short_url"`
	// Reason 是 phishing、malware、spam、impersonation 或 other
	Reason string `json:"reason"`
	// Details 是可选补充说明
	Details string `json:"details"`
}

type createReportResponse struct {
	// ID 是 ULID 举报标识
	ID string `json:"id"`
	// Status 是举报初始状态
	Status uint8 `json:"status"`
}

type resolveReportRequest struct {
	// Decision 是 takedown 或 reject
	Decision string `json:"decision"`
	// Note 是管理员处理说明
	Note string `json:"note"`
}

type addBlockedDomainRequest struct {
	// Hostname 是准备阻断的域名或 IP
	Hostname string `json:"hostname"`
	// Reason 是阻断原因
	Reason string `json:"reason"`
}

type reportResponse struct {
	// ID 是 ULID 举报标识
	ID string `json:"id"`
	// LinkID 是被举报短链 ULID
	LinkID string `json:"link_id"`
	// LinkTitle 是短链标题
	LinkTitle string `json:"link_title"`
	// LinkSlug 是短码
	LinkSlug string `json:"link_slug"`
	// DomainHostname 是短链域名
	DomainHostname string `json:"domain_hostname"`
	// DestinationURL 是目标地址
	DestinationURL string `json:"destination_url"`
	// LinkStatus 是短链当前状态
	LinkStatus uint8 `json:"link_status"`
	// Reason 是举报类型
	Reason string `json:"reason"`
	// Details 是举报说明
	Details string `json:"details"`
	// Status 是举报状态
	Status uint8 `json:"status"`
	// ResolutionNote 是处理说明
	ResolutionNote string `json:"resolution_note"`
	// ResolvedAt 是处理完成 UTC 时间
	ResolvedAt *time.Time `json:"resolved_at"`
	// CreatedAt 是举报提交 UTC 时间
	CreatedAt time.Time `json:"created_at"`
}

type reportListResponse struct {
	// Items 是举报队列
	Items []reportResponse `json:"items"`
}

type blockedDomainResponse struct {
	// ID 是 ULID 黑名单记录标识
	ID string `json:"id"`
	// Hostname 是阻断域名或 IP
	Hostname string `json:"hostname"`
	// Reason 是阻断原因
	Reason string `json:"reason"`
	// CreatedAt 是规则创建 UTC 时间
	CreatedAt time.Time `json:"created_at"`
}

type blockedDomainListResponse struct {
	// Items 是有效黑名单规则
	Items []blockedDomainResponse `json:"items"`
}

// CreateReport 接收公开访客提交的滥用举报
func (h *Handler) CreateReport(c *gin.Context) {
	var request createReportRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "举报请求参数无效")
		return
	}
	result, err := h.service.CreateReport(
		c.Request.Context(),
		request.ShortURL,
		request.Reason,
		request.Details,
	)
	if err != nil {
		h.writeServiceError(c, err, "提交滥用举报失败")
		return
	}
	c.JSON(http.StatusCreated, createReportResponse{
		ID:     result.ID,
		Status: result.Status,
	})
}

// ListReports 返回管理员举报队列
func (h *Handler) ListReports(c *gin.Context) {
	status := ReportStatusPending
	if rawStatus := c.Query("status"); rawStatus != "" {
		parsed, err := strconv.ParseUint(rawStatus, 10, 8)
		if err != nil {
			h.writeError(c, http.StatusBadRequest, "INVALID_STATUS", "status 必须为数字")
			return
		}
		status = uint8(parsed)
	}
	limit := 20
	if rawLimit := c.Query("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil {
			h.writeError(c, http.StatusBadRequest, "INVALID_LIMIT", "limit 必须为数字")
			return
		}
		limit = parsed
	}
	reports, err := h.service.ListReports(c.Request.Context(), status, limit)
	if err != nil {
		h.writeServiceError(c, err, "查询滥用举报失败")
		return
	}
	items := make([]reportResponse, 0, len(reports))
	for index := range reports {
		items = append(items, toReportResponse(reports[index]))
	}
	c.JSON(http.StatusOK, reportListResponse{Items: items})
}

// ResolveReport 处理指定举报
func (h *Handler) ResolveReport(c *gin.Context) {
	var request resolveReportRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "举报处理参数无效")
		return
	}
	result, err := h.service.ResolveReport(
		c.Request.Context(),
		c.Param("id"),
		c.GetString(identity.ContextUserID),
		request.Decision,
		request.Note,
	)
	if err != nil {
		h.writeServiceError(c, err, "处理滥用举报失败")
		return
	}
	c.JSON(http.StatusOK, toReportResponse(result.Report))
}

// ListBlockedDomains 返回有效目标域名黑名单
func (h *Handler) ListBlockedDomains(c *gin.Context) {
	domains, err := h.service.ListBlockedDomains(c.Request.Context())
	if err != nil {
		h.writeServiceError(c, err, "查询域名黑名单失败")
		return
	}
	items := make([]blockedDomainResponse, 0, len(domains))
	for index := range domains {
		items = append(items, blockedDomainResponse{
			ID:        domains[index].ID,
			Hostname:  domains[index].Hostname,
			Reason:    domains[index].Reason,
			CreatedAt: domains[index].CreatedAt,
		})
	}
	c.JSON(http.StatusOK, blockedDomainListResponse{Items: items})
}

// AddBlockedDomain 添加目标域名黑名单规则
func (h *Handler) AddBlockedDomain(c *gin.Context) {
	var request addBlockedDomainRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "黑名单参数无效")
		return
	}
	domain, err := h.service.AddBlockedDomain(
		c.Request.Context(),
		c.GetString(identity.ContextUserID),
		request.Hostname,
		request.Reason,
	)
	if err != nil {
		h.writeServiceError(c, err, "添加域名黑名单失败")
		return
	}
	c.JSON(http.StatusCreated, blockedDomainResponse{
		ID:        domain.ID,
		Hostname:  domain.Hostname,
		Reason:    domain.Reason,
		CreatedAt: domain.CreatedAt,
	})
}

// RemoveBlockedDomain 停用目标域名黑名单规则
func (h *Handler) RemoveBlockedDomain(c *gin.Context) {
	err := h.service.RemoveBlockedDomain(
		c.Request.Context(),
		c.GetString(identity.ContextUserID),
		c.Param("id"),
	)
	if err != nil {
		h.writeServiceError(c, err, "移除域名黑名单失败")
		return
	}
	c.Status(http.StatusNoContent)
}

// writeServiceError 将治理领域错误映射为稳定 HTTP 响应
func (h *Handler) writeServiceError(c *gin.Context, err error, logMessage string) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		h.writeError(c, http.StatusBadRequest, "INVALID_ABUSE_REQUEST", err.Error())
	case errors.Is(err, ErrNotFound):
		h.writeError(c, http.StatusNotFound, "ABUSE_NOT_FOUND", "举报或短链不存在")
	case errors.Is(err, ErrAlreadyResolved):
		h.writeError(c, http.StatusConflict, "ABUSE_ALREADY_RESOLVED", "举报已经处理")
	default:
		h.logger.Error(logMessage, "error", err)
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

// toReportResponse 将举报模型转换为管理员 DTO
func toReportResponse(report Report) reportResponse {
	return reportResponse{
		ID:             report.ID,
		LinkID:         report.LinkID,
		LinkTitle:      report.LinkTitle,
		LinkSlug:       report.LinkSlug,
		DomainHostname: report.DomainHostname,
		DestinationURL: report.DestinationURL,
		LinkStatus:     report.LinkStatus,
		Reason:         report.Reason,
		Details:        report.Details,
		Status:         report.Status,
		ResolutionNote: report.ResolutionNote,
		ResolvedAt:     report.ResolvedAt,
		CreatedAt:      report.CreatedAt,
	}
}
