package trial

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/PYang176/Short-URL-Service/internal/identity"
	"github.com/PYang176/Short-URL-Service/internal/link"
)

// Handler 提供无需登录的游客短链创建和列表接口
type Handler struct {
	// sessions 管理游客 Cookie 会话和配额
	sessions *Service
	// links 复用正式短链的创建、列表和公开地址生成逻辑
	links *link.Service
	// logger 记录内部错误
	logger *slog.Logger
}

// NewHandler 创建游客试用 HTTP 处理器
func NewHandler(sessions *Service, links *link.Service, logger *slog.Logger) *Handler {
	return &Handler{sessions: sessions, links: links, logger: logger}
}

type createLinkRequest struct {
	// DestinationURL 是游客希望跳转的目标地址
	DestinationURL string `json:"destination_url"`
	// Alias 是可选自定义短码
	Alias string `json:"alias"`
	// Title 是可选链接标题
	Title string `json:"title"`
	// ExpiresInMinutes 是 15、30、60、720 或 1440 分钟
	ExpiresInMinutes int `json:"expires_in_minutes"`
}

type trialLinkResponse struct {
	// ID 是 ULID 链接标识
	ID string `json:"id"`
	// Slug 是短链路径中的短码
	Slug string `json:"slug"`
	// ShortURL 是可立即访问的完整短链
	ShortURL string `json:"short_url"`
	// DestinationURL 是规范化后的目标地址
	DestinationURL string `json:"destination_url"`
	// Title 是链接标题
	Title string `json:"title"`
	// ExpiresAt 是链接失效的 UTC 时间
	ExpiresAt time.Time `json:"expires_at"`
	// CreatedAt 是链接创建的 UTC 时间
	CreatedAt time.Time `json:"created_at"`
}

type linkListResponse struct {
	// Items 是当前游客会话创建的链接
	Items []trialLinkResponse `json:"items"`
	// Remaining 是当前会话仍可创建的链接数量
	Remaining int `json:"remaining"`
	// Limit 是当前会话允许创建的链接上限
	Limit int `json:"limit"`
	// ExpiresAt 是游客会话失效时间；无有效会话时为空
	ExpiresAt *time.Time `json:"expires_at"`
}

type claimResponse struct {
	// ClaimedLinks 是迁移到正式工作空间的链接数量
	ClaimedLinks int64 `json:"claimed_links"`
	// ClaimedClicks 是迁移到正式工作空间的点击明细数量
	ClaimedClicks int64 `json:"claimed_clicks"`
}

// Create 创建不绑定登录账号的短期短链接
func (h *Handler) Create(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "读取请求体失败")
		return
	}
	var request createLinkRequest
	if err := decodeJSON(body, &request); err != nil {
		h.writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "创建短链请求参数无效")
		return
	}
	request.DestinationURL = strings.TrimSpace(request.DestinationURL)
	request.Alias = strings.TrimSpace(request.Alias)
	request.Title = strings.TrimSpace(request.Title)
	if request.DestinationURL == "" {
		h.writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "目标地址不能为空")
		return
	}

	session, err := h.ensureSession(c)
	if err != nil {
		h.logger.Error("创建游客会话失败", "error", err)
		h.writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "服务器内部错误")
		return
	}
	expiryMinutes := request.ExpiresInMinutes
	if expiryMinutes == 0 {
		expiryMinutes = ExpiryMinutes1440
	}
	expiresAt, err := h.sessions.LinkExpiresAt(session, expiryMinutes)
	if err != nil {
		if errors.Is(err, ErrInvalidExpiry) {
			h.writeError(
				c,
				http.StatusBadRequest,
				"INVALID_TRIAL_EXPIRY",
				"有效期必须是 15、30、60、720 或 1440 分钟",
			)
			return
		}
		h.logger.Error("计算试用链接有效期失败", "error", err)
		h.writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "服务器内部错误")
		return
	}
	if err := h.sessions.ReserveLink(c.Request.Context(), session.ID); err != nil {
		if errors.Is(err, ErrLinkLimitReached) {
			h.writeError(c, http.StatusTooManyRequests, "TRIAL_LINK_LIMIT_REACHED", "本次试用已创建全部短链")
			return
		}
		h.logger.Error("占用游客链接配额失败", "error", err)
		h.writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "服务器内部错误")
		return
	}

	result, err := h.links.Create(c.Request.Context(), session.WorkspaceID, link.CreateInput{
		DestinationURL: request.DestinationURL,
		Alias:          request.Alias,
		Title:          request.Title,
		RedirectCode:   http.StatusFound,
		ExpiresAt:      &expiresAt,
	})
	if err != nil {
		if releaseErr := h.sessions.ReleaseLink(c.Request.Context(), session.ID); releaseErr != nil {
			h.logger.Warn("归还游客链接配额失败", "error", releaseErr)
		}
		h.writeLinkError(c, err)
		return
	}

	c.JSON(http.StatusCreated, h.toResponse(result))
}

// List 返回当前 Cookie 会话创建的短期链接
func (h *Handler) List(c *gin.Context) {
	session, err := h.sessionFromCookie(c)
	if err != nil {
		if IsInvalidSession(err) {
			c.JSON(http.StatusOK, linkListResponse{
				Items:     []trialLinkResponse{},
				Remaining: h.sessions.MaxLinks(),
				Limit:     h.sessions.MaxLinks(),
			})
			return
		}
		h.logger.Error("恢复游客会话失败", "error", err)
		h.writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "服务器内部错误")
		return
	}

	links, err := h.links.List(
		c.Request.Context(),
		session.WorkspaceID,
		h.sessions.MaxLinks(),
		nil,
		"",
		link.ListFilter{},
	)
	if err != nil {
		h.logger.Error("查询游客短链失败", "error", err)
		h.writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "服务器内部错误")
		return
	}
	items := make([]trialLinkResponse, 0, len(links))
	for index := range links {
		items = append(items, h.toResponse(&links[index]))
	}
	expiresAt := session.ExpiresAt
	c.JSON(http.StatusOK, linkListResponse{
		Items:     items,
		Remaining: h.sessions.Remaining(session),
		Limit:     h.sessions.MaxLinks(),
		ExpiresAt: &expiresAt,
	})
}

// Claim 将当前 Cookie 游客会话中的链接认领到登录工作空间
func (h *Handler) Claim(c *gin.Context) {
	token, err := c.Cookie(CookieName)
	if err != nil {
		c.JSON(http.StatusOK, claimResponse{})
		return
	}

	result, err := h.sessions.Claim(
		c.Request.Context(),
		token,
		c.GetString(identity.ContextWorkspaceID),
	)
	if err != nil {
		switch {
		case IsInvalidSession(err):
			h.clearCookie(c)
			c.JSON(http.StatusOK, claimResponse{})
		case errors.Is(err, ErrClaimConflict):
			h.writeError(c, http.StatusConflict, "TRIAL_CLAIM_CONFLICT", "游客链接与当前工作空间中的短码冲突")
		case errors.Is(err, ErrClaimTargetNotFound):
			h.writeError(c, http.StatusConflict, "TRIAL_CLAIM_TARGET_INVALID", "当前工作空间无法接收游客链接")
		default:
			h.logger.Error("认领游客链接失败", "error", err)
			h.writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "服务器内部错误")
		}
		return
	}

	for _, key := range result.CacheKeys {
		h.links.Invalidate(c.Request.Context(), key.DomainID, key.Slug)
	}
	h.clearCookie(c)
	c.JSON(http.StatusOK, claimResponse{
		ClaimedLinks:  result.Links,
		ClaimedClicks: result.Clicks,
	})
}

// ensureSession 恢复有效 Cookie；不存在或已过期时创建新的游客会话
func (h *Handler) ensureSession(c *gin.Context) (*Session, error) {
	session, err := h.sessionFromCookie(c)
	if err == nil {
		return session, nil
	}
	if !IsInvalidSession(err) {
		return nil, err
	}

	session, token, err := h.sessions.Create(c.Request.Context())
	if err != nil {
		return nil, err
	}
	h.setCookie(c, token)
	return session, nil
}

// sessionFromCookie 从 HttpOnly Cookie 恢复游客会话
func (h *Handler) sessionFromCookie(c *gin.Context) (*Session, error) {
	token, err := c.Cookie(CookieName)
	if err != nil {
		return nil, ErrSessionNotFound
	}
	return h.sessions.Resume(c.Request.Context(), token)
}

// setCookie 写入不暴露给 JavaScript 的游客会话 Cookie
func (h *Handler) setCookie(c *gin.Context, token string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(
		CookieName,
		token,
		int(h.sessions.SessionTTL().Seconds()),
		"/",
		"",
		h.cookieSecure(c),
		true,
	)
}

// clearCookie 删除浏览器中的游客会话 Cookie
func (h *Handler) clearCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(CookieName, "", -1, "/", "", h.cookieSecure(c), true)
}

// cookieSecure 根据直接 TLS 或可信代理协议判断 Cookie 是否需要 Secure
func (h *Handler) cookieSecure(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	forwardedProto := strings.TrimSpace(strings.Split(c.GetHeader("X-Forwarded-Proto"), ",")[0])
	return strings.EqualFold(forwardedProto, "https")
}

// toResponse 将链接模型转换为游客接口 DTO
func (h *Handler) toResponse(value *link.Link) trialLinkResponse {
	expiresAt := time.Time{}
	if value.ExpiresAt != nil {
		expiresAt = *value.ExpiresAt
	}
	return trialLinkResponse{
		ID:             value.ID,
		Slug:           value.Slug,
		ShortURL:       h.links.ShortURL(value),
		DestinationURL: value.DestinationURL,
		Title:          value.Title,
		ExpiresAt:      expiresAt,
		CreatedAt:      value.CreatedAt,
	}
}

// decodeJSON 严格解析游客创建请求并拒绝未知字段
func decodeJSON(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

// writeLinkError 将链接领域错误映射为游客接口响应
func (h *Handler) writeLinkError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, link.ErrInvalidInput):
		h.writeError(c, http.StatusBadRequest, "INVALID_LINK", "短链参数无效")
	case errors.Is(err, link.ErrSlugTaken):
		h.writeError(c, http.StatusConflict, "SLUG_TAKEN", "短码已被占用")
	default:
		h.logger.Error("创建游客短链失败", "error", err)
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
