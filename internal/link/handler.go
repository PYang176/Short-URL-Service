package link

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/PYang176/Short-URL-Service/internal/identity"
)

// Handler 提供短链接管理和公开跳转 HTTP 接口
type Handler struct {
	service *Service
}

// NewHandler 创建短链接 HTTP 处理器
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

type createRequest struct {
	// DestinationURL 是准备缩短的目标地址
	DestinationURL string `json:"destination_url" binding:"required"`
	// Alias 是可选自定义短码
	Alias string `json:"alias"`
	// Title 是可选链接标题
	Title string `json:"title"`
	// Notes 是可选备注
	Notes string `json:"notes"`
	// RedirectCode 是 301 或 302，默认 302
	RedirectCode int `json:"redirect_code"`
	// StartsAt 是可选生效时间
	StartsAt *time.Time `json:"starts_at"`
	// ExpiresAt 是可选过期时间
	ExpiresAt *time.Time `json:"expires_at"`
}

type updateRequest struct {
	// DestinationURL 是可选的新目标地址
	DestinationURL *string `json:"destination_url"`
	// Title 是可选的新标题
	Title *string `json:"title"`
	// Notes 是可选的新备注
	Notes *string `json:"notes"`
	// Status 是可选的新状态
	Status *uint8 `json:"status"`
	// RedirectCode 是可选的新跳转状态码
	RedirectCode *int `json:"redirect_code"`
	// ExpiresAt 是可选的新过期时间
	ExpiresAt *time.Time `json:"expires_at"`
	// ClearExpiresAt 表示删除已有过期时间
	ClearExpiresAt bool `json:"clear_expires_at"`
}

type linkResponse struct {
	// ID 是 ULID 链接标识
	ID string `json:"id"`
	// Slug 是大小写敏感短码
	Slug string `json:"slug"`
	// ShortURL 是完整短链地址
	ShortURL string `json:"short_url"`
	// DestinationURL 是当前目标地址
	DestinationURL string `json:"destination_url"`
	// Title 是链接标题
	Title string `json:"title"`
	// Notes 是链接备注
	Notes string `json:"notes"`
	// Status 是链接状态
	Status uint8 `json:"status"`
	// RedirectCode 是 301 或 302
	RedirectCode int `json:"redirect_code"`
	// StartsAt 是可选生效时间
	StartsAt *time.Time `json:"starts_at"`
	// ExpiresAt 是可选过期时间
	ExpiresAt *time.Time `json:"expires_at"`
	// CreatedAt 是链接创建时间
	CreatedAt time.Time `json:"created_at"`
}

type listResponse struct {
	// Items 是当前页链接
	Items []linkResponse `json:"items"`
	// NextCursor 是下一页游标，空值表示没有更多数据
	NextCursor string `json:"next_cursor"`
}

// Create 创建当前工作空间下的短链接
func (h *Handler) Create(c *gin.Context) {
	var request createRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "创建短链请求参数无效")
		return
	}
	link, err := h.service.Create(
		c.Request.Context(),
		c.GetString(identity.ContextWorkspaceID),
		CreateInput(request),
	)
	if err != nil {
		writeLinkError(c, err)
		return
	}
	c.JSON(http.StatusCreated, h.toResponse(link))
}

// List 返回当前工作空间的短链接列表
func (h *Handler) List(c *gin.Context) {
	limit := 20
	if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil {
			writeError(c, http.StatusBadRequest, "INVALID_CURSOR", "limit 必须为数字")
			return
		}
		limit = parsed
	}
	beforeCreatedAt, beforeID, err := decodeCursor(c.Query("cursor"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_CURSOR", "分页游标无效")
		return
	}

	links, err := h.service.List(
		c.Request.Context(),
		c.GetString(identity.ContextWorkspaceID),
		limit,
		beforeCreatedAt,
		beforeID,
	)
	if err != nil {
		writeLinkError(c, err)
		return
	}

	items := make([]linkResponse, 0, len(links))
	for index := range links {
		items = append(items, h.toResponse(&links[index]))
	}
	nextCursor := ""
	if len(links) == limit && len(links) > 0 {
		nextCursor = encodeCursor(links[len(links)-1])
	}
	c.JSON(http.StatusOK, listResponse{Items: items, NextCursor: nextCursor})
}

// Get 返回当前工作空间内的单个短链接
func (h *Handler) Get(c *gin.Context) {
	link, err := h.service.Get(
		c.Request.Context(),
		c.GetString(identity.ContextWorkspaceID),
		c.Param("id"),
	)
	if err != nil {
		writeLinkError(c, err)
		return
	}
	c.JSON(http.StatusOK, h.toResponse(link))
}

// Update 修改当前工作空间内的短链接
func (h *Handler) Update(c *gin.Context) {
	var request updateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "更新短链请求参数无效")
		return
	}
	link, err := h.service.Update(
		c.Request.Context(),
		c.GetString(identity.ContextWorkspaceID),
		c.Param("id"),
		UpdateInput(request),
	)
	if err != nil {
		writeLinkError(c, err)
		return
	}
	c.JSON(http.StatusOK, h.toResponse(link))
}

// Delete 软删除当前工作空间内的短链接
func (h *Handler) Delete(c *gin.Context) {
	if err := h.service.Delete(
		c.Request.Context(),
		c.GetString(identity.ContextWorkspaceID),
		c.Param("id"),
	); err != nil {
		writeLinkError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// Redirect 解析短码并返回 HTTP 跳转
func (h *Handler) Redirect(c *gin.Context) {
	destination, redirectCode, err := h.service.Resolve(c.Request.Context(), c.Param("slug"))
	if err != nil {
		writeLinkError(c, err)
		return
	}
	http.Redirect(c.Writer, c.Request, destination, redirectCode)
}

// toResponse 将链接模型转换为 HTTP DTO
func (h *Handler) toResponse(link *Link) linkResponse {
	return linkResponse{
		ID:             link.ID,
		Slug:           link.Slug,
		ShortURL:       h.service.ShortURL(link),
		DestinationURL: link.DestinationURL,
		Title:          link.Title,
		Notes:          link.Notes,
		Status:         link.Status,
		RedirectCode:   link.RedirectCode,
		StartsAt:       link.StartsAt,
		ExpiresAt:      link.ExpiresAt,
		CreatedAt:      link.CreatedAt,
	}
}

// encodeCursor 将链接创建时间和 ID 编码为不透明分页游标
func encodeCursor(link Link) string {
	value := link.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + link.ID
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

// decodeCursor 解析分页游标并返回创建时间和链接 ID
func decodeCursor(cursor string) (*time.Time, string, error) {
	if strings.TrimSpace(cursor) == "" {
		return nil, "", nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, "", err
	}
	parts := strings.SplitN(string(decoded), "|", 2)
	if len(parts) != 2 || parts[1] == "" {
		return nil, "", ErrInvalidInput
	}
	createdAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, "", err
	}
	return &createdAt, parts[1], nil
}

// writeLinkError 将短链领域错误映射为 HTTP 响应
func writeLinkError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		writeError(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
	case errors.Is(err, ErrSlugTaken):
		writeError(c, http.StatusConflict, "SLUG_TAKEN", "短码已被占用")
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrNotStarted):
		writeError(c, http.StatusNotFound, "LINK_NOT_FOUND", "短链不存在")
	case errors.Is(err, ErrGone):
		writeError(c, http.StatusGone, "LINK_GONE", "短链已失效")
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
