package domain

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/PYang176/Short-URL-Service/internal/identity"
)

// Handler 提供自定义域名 HTTP 接口
type Handler struct {
	// service 提供域名业务能力
	service *Service
}

// NewHandler 创建域名 HTTP 处理器
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

type addRequest struct {
	// Hostname 是用户输入的自定义域名
	Hostname string `json:"hostname" binding:"required"`
}

type domainResponse struct {
	// ID 是 ULID 域名标识
	ID string `json:"id"`
	// Hostname 是自定义域名
	Hostname string `json:"hostname"`
	// Status 是验证状态
	Status uint8 `json:"status"`
	// Verified 表示域名是否可以创建短链
	Verified bool `json:"verified"`
	// TXTName 是验证 TXT 的记录名
	TXTName string `json:"txt_name"`
	// TXTValue 是验证 TXT 的记录值
	TXTValue string `json:"txt_value"`
	// CNAME 是域名需要指向的平台接入地址
	CNAME string `json:"cname"`
	// LastVerifiedAt 是最近验证时间
	LastVerifiedAt *time.Time `json:"last_verified_at"`
	// CreatedAt 是添加时间
	CreatedAt time.Time `json:"created_at"`
}

// List 返回当前工作空间的自定义域名
func (h *Handler) List(c *gin.Context) {
	domains, err := h.service.List(c.Request.Context(), c.GetString(identity.ContextWorkspaceID))
	if err != nil {
		writeDomainError(c, err)
		return
	}
	items := make([]domainResponse, 0, len(domains))
	for index := range domains {
		items = append(items, toResponse(&domains[index]))
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// Add 添加待验证域名
func (h *Handler) Add(c *gin.Context) {
	var request addRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeDomainError(c, ErrInvalidDomain)
		return
	}
	result, err := h.service.Add(
		c.Request.Context(),
		c.GetString(identity.ContextWorkspaceID),
		request.Hostname,
	)
	if err != nil {
		writeDomainError(c, err)
		return
	}
	c.JSON(http.StatusCreated, toResponse(result))
}

// Verify 验证域名 DNS 配置
func (h *Handler) Verify(c *gin.Context) {
	result, err := h.service.Verify(
		c.Request.Context(),
		c.GetString(identity.ContextWorkspaceID),
		c.Param("id"),
	)
	if err != nil {
		writeDomainError(c, err)
		return
	}
	c.JSON(http.StatusOK, toResponse(result))
}

// Delete 删除尚未被链接使用的域名
func (h *Handler) Delete(c *gin.Context) {
	if err := h.service.Delete(
		c.Request.Context(),
		c.GetString(identity.ContextWorkspaceID),
		c.Param("id"),
	); err != nil {
		writeDomainError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// toResponse 将域名模型转换为 HTTP DTO
func toResponse(value *Domain) domainResponse {
	return domainResponse{
		ID:             value.ID,
		Hostname:       value.Hostname,
		Status:         value.Status,
		Verified:       value.Status == StatusVerified,
		TXTName:        "_shorturl." + value.Hostname,
		TXTValue:       value.VerificationToken,
		CNAME:          value.ExpectedCNAME,
		LastVerifiedAt: value.LastVerifiedAt,
		CreatedAt:      value.CreatedAt,
	}
}

// writeDomainError 将域名错误映射为 HTTP 响应
func writeDomainError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidDomain):
		writeError(c, http.StatusBadRequest, "INVALID_DOMAIN", "域名格式无效")
	case errors.Is(err, ErrDomainExists):
		writeError(c, http.StatusConflict, "DOMAIN_EXISTS", "域名已被添加")
	case errors.Is(err, ErrNotFound):
		writeError(c, http.StatusNotFound, "DOMAIN_NOT_FOUND", "域名不存在")
	case errors.Is(err, ErrNotVerified):
		writeError(c, http.StatusConflict, "DOMAIN_NOT_VERIFIED", "域名尚未验证")
	case errors.Is(err, ErrVerificationFailed):
		writeError(c, http.StatusBadRequest, "DOMAIN_VERIFY_FAILED", "TXT 或 CNAME 记录未通过验证")
	case errors.Is(err, ErrDomainInUse):
		writeError(c, http.StatusConflict, "DOMAIN_IN_USE", "请先删除使用该域名的短链")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "服务器内部错误")
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
