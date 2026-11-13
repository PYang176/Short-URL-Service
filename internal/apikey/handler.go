package apikey

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/PYang176/Short-URL-Service/internal/identity"
)

// Handler 提供 API Key 创建、列表和撤销接口
type Handler struct {
	// service 管理 API Key 生命周期
	service *Service
	// logger 记录内部错误
	logger *slog.Logger
}

// NewHandler 创建 API Key HTTP 处理器
func NewHandler(service *Service, logger *slog.Logger) *Handler {
	return &Handler{service: service, logger: logger}
}

type createRequest struct {
	// Name 是用户可读的 Key 名称
	Name string `json:"name"`
	// Scopes 是请求授予的作用域
	Scopes []string `json:"scopes"`
	// ExpiresAt 是可选失效时间
	ExpiresAt *time.Time `json:"expires_at"`
}

type keyResponse struct {
	// ID 是 API Key 记录 ULID
	ID string `json:"id"`
	// Name 是用户可读名称
	Name string `json:"name"`
	// Prefix 是可展示前缀
	Prefix string `json:"prefix"`
	// Scopes 是授予作用域
	Scopes []string `json:"scopes"`
	// LastUsedAt 是最近认证时间
	LastUsedAt *time.Time `json:"last_used_at"`
	// ExpiresAt 是可选失效时间
	ExpiresAt *time.Time `json:"expires_at"`
	// RevokedAt 是撤销时间
	RevokedAt *time.Time `json:"revoked_at"`
	// CreatedAt 是创建时间
	CreatedAt time.Time `json:"created_at"`
}

type createResponse struct {
	// Key 是 API Key 元数据
	Key keyResponse `json:"key"`
	// Secret 是只返回一次的完整 API Key
	Secret string `json:"secret"`
}

type listResponse struct {
	// Items 是当前工作空间 API Key 元数据
	Items []keyResponse `json:"items"`
}

// Create 创建新的 API Key
func (h *Handler) Create(c *gin.Context) {
	var request createRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "创建 API Key 请求参数无效")
		return
	}
	result, err := h.service.Create(
		c.Request.Context(),
		c.GetString(identity.ContextWorkspaceID),
		c.GetString(identity.ContextUserID),
		request.Name,
		request.Scopes,
		request.ExpiresAt,
	)
	if err != nil {
		if errors.Is(err, ErrInvalidInput) {
			h.writeError(c, http.StatusBadRequest, "INVALID_API_KEY", "API Key 参数无效")
			return
		}
		h.logger.Error("创建 API Key 失败", "error", err)
		h.writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "服务器内部错误")
		return
	}
	c.JSON(http.StatusCreated, createResponse{
		Key:    toResponse(result.Key),
		Secret: result.Plaintext,
	})
}

// List 返回当前工作空间的 API Key 元数据
func (h *Handler) List(c *gin.Context) {
	keys, err := h.service.List(
		c.Request.Context(),
		c.GetString(identity.ContextWorkspaceID),
	)
	if err != nil {
		h.logger.Error("查询 API Key 列表失败", "error", err)
		h.writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "服务器内部错误")
		return
	}
	items := make([]keyResponse, 0, len(keys))
	for index := range keys {
		items = append(items, toResponse(keys[index]))
	}
	c.JSON(http.StatusOK, listResponse{Items: items})
}

// Revoke 撤销当前工作空间的 API Key
func (h *Handler) Revoke(c *gin.Context) {
	err := h.service.Revoke(
		c.Request.Context(),
		c.GetString(identity.ContextWorkspaceID),
		c.Param("id"),
	)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			h.writeError(c, http.StatusNotFound, "API_KEY_NOT_FOUND", "API Key 不存在")
			return
		}
		h.logger.Error("撤销 API Key 失败", "error", err)
		h.writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "服务器内部错误")
		return
	}
	c.Status(http.StatusNoContent)
}

// toResponse 将 API Key 模型转换为安全 DTO
func toResponse(key Key) keyResponse {
	return keyResponse{
		ID:         key.ID,
		Name:       key.Name,
		Prefix:     key.KeyPrefix,
		Scopes:     splitScopes(key.Scopes),
		LastUsedAt: key.LastUsedAt,
		ExpiresAt:  key.ExpiresAt,
		RevokedAt:  key.RevokedAt,
		CreatedAt:  key.CreatedAt,
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
