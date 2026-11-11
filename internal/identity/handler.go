package identity

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Handler 提供注册、登录、Token 轮换和资料管理 HTTP 接口
type Handler struct {
	service *Service
}

// NewHandler 创建身份 HTTP 处理器
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

type registerRequest struct {
	// Email 是注册邮箱
	Email string `json:"email" binding:"required"`
	// Password 是首次设置的明文密码
	Password string `json:"password" binding:"required"`
	// DisplayName 是用户展示名称
	DisplayName string `json:"display_name" binding:"required"`
}

type loginRequest struct {
	// Email 是登录邮箱
	Email string `json:"email" binding:"required"`
	// Password 是待校验的明文密码
	Password string `json:"password" binding:"required"`
}

type refreshRequest struct {
	// RefreshToken 是客户端持有的原始轮换凭证
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type updateProfileRequest struct {
	// DisplayName 是准备更新的展示名称
	DisplayName string `json:"display_name" binding:"required"`
}

type userResponse struct {
	// ID 是 ULID 用户标识
	ID string `json:"id"`
	// Email 是用户登录邮箱
	Email string `json:"email"`
	// DisplayName 是用户展示名称
	DisplayName string `json:"display_name"`
	// Role 是平台角色
	Role uint8 `json:"role"`
	// CreatedAt 是账号创建的 UTC 时间
	CreatedAt time.Time `json:"created_at"`
}

type workspaceResponse struct {
	// ID 是 ULID 工作空间标识
	ID string `json:"id"`
	// Name 是工作空间展示名称
	Name string `json:"name"`
	// Slug 是工作空间唯一短标识
	Slug string `json:"slug"`
	// CreatedAt 是工作空间创建的 RFC 3339 时间
	CreatedAt string `json:"created_at"`
}

type tokenResponse struct {
	// AccessToken 是访问受保护接口的 JWT
	AccessToken string `json:"access_token"`
	// TokenType 是 HTTP Authorization 使用的凭证类型
	TokenType string `json:"token_type"`
	// ExpiresAt 是 Access Token 的 UTC 过期时间
	ExpiresAt time.Time `json:"expires_at"`
	// RefreshToken 是下一次轮换使用的原始凭证
	RefreshToken string `json:"refresh_token"`
}

type authResponse struct {
	// User 是脱敏后的用户信息
	User userResponse `json:"user"`
	// Workspace 是当前工作空间
	Workspace workspaceResponse `json:"workspace"`
	// Token 是登录凭证
	Token tokenResponse `json:"token"`
}

type profileResponse struct {
	// User 是脱敏后的用户信息
	User userResponse `json:"user"`
	// Workspace 是当前工作空间
	Workspace workspaceResponse `json:"workspace"`
}

// Register 处理注册请求并返回首组登录凭证
func (h *Handler) Register(c *gin.Context) {
	var request registerRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "注册请求参数无效")
		return
	}

	result, err := h.service.Register(c.Request.Context(), request.Email, request.Password, request.DisplayName)
	if err != nil {
		writeIdentityError(c, err)
		return
	}
	c.JSON(http.StatusCreated, toAuthResponse(result))
}

// Login 校验账号密码并返回新的登录凭证
func (h *Handler) Login(c *gin.Context) {
	var request loginRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "登录请求参数无效")
		return
	}

	result, err := h.service.Login(c.Request.Context(), request.Email, request.Password)
	if err != nil {
		writeIdentityError(c, err)
		return
	}
	c.JSON(http.StatusOK, toAuthResponse(result))
}

// Refresh 轮换 Refresh Token 并返回新的凭证对
func (h *Handler) Refresh(c *gin.Context) {
	var request refreshRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "refresh_token 不能为空")
		return
	}

	result, err := h.service.Refresh(c.Request.Context(), request.RefreshToken)
	if err != nil {
		writeIdentityError(c, err)
		return
	}
	c.JSON(http.StatusOK, toAuthResponse(result))
}

// Logout 撤销客户端提交的 Refresh Token
func (h *Handler) Logout(c *gin.Context) {
	var request refreshRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "refresh_token 不能为空")
		return
	}
	if err := h.service.Logout(c.Request.Context(), request.RefreshToken); err != nil {
		writeIdentityError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// Profile 返回当前用户和默认工作空间
func (h *Handler) Profile(c *gin.Context) {
	userID := c.GetString(ContextUserID)
	user, workspace, err := h.service.Profile(c.Request.Context(), userID)
	if err != nil {
		writeIdentityError(c, err)
		return
	}
	c.JSON(http.StatusOK, toProfileResponse(user, workspace))
}

// UpdateProfile 更新当前用户展示名称
func (h *Handler) UpdateProfile(c *gin.Context) {
	var request updateProfileRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "display_name 不能为空")
		return
	}

	user, workspace, err := h.service.UpdateDisplayName(
		c.Request.Context(),
		c.GetString(ContextUserID),
		request.DisplayName,
	)
	if err != nil {
		writeIdentityError(c, err)
		return
	}
	c.JSON(http.StatusOK, toProfileResponse(user, workspace))
}

// toAuthResponse 将业务结果转换为不暴露密码摘要的认证 DTO
func toAuthResponse(result *AuthResult) authResponse {
	return authResponse{
		User:      toUserResponse(result.User),
		Workspace: toWorkspaceResponse(result.Workspace),
		Token: tokenResponse{
			AccessToken:  result.AccessToken,
			TokenType:    "Bearer",
			ExpiresAt:    result.AccessTokenExpires,
			RefreshToken: result.RefreshToken,
		},
	}
}

// toProfileResponse 将用户和工作空间转换为资料 DTO
func toProfileResponse(user *User, workspace *Workspace) profileResponse {
	return profileResponse{
		User:      toUserResponse(*user),
		Workspace: toWorkspaceResponse(*workspace),
	}
}

// toUserResponse 将用户模型转换为脱敏响应
func toUserResponse(user User) userResponse {
	return userResponse{
		ID:          user.ID,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		Role:        user.Role,
		CreatedAt:   user.CreatedAt,
	}
}

// toWorkspaceResponse 将工作空间模型转换为响应
func toWorkspaceResponse(workspace Workspace) workspaceResponse {
	return workspaceResponse{
		ID:        workspace.ID,
		Name:      workspace.Name,
		Slug:      workspace.Slug,
		CreatedAt: workspace.CreatedAt.Format(time.RFC3339Nano),
	}
}

// writeIdentityError 将身份领域错误映射为稳定 HTTP 状态和错误码
func writeIdentityError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		writeError(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
	case errors.Is(err, ErrEmailExists):
		writeError(c, http.StatusConflict, "EMAIL_EXISTS", "该邮箱已注册")
	case errors.Is(err, ErrInvalidCredentials):
		writeError(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "邮箱或密码错误")
	case errors.Is(err, ErrTokenReuse):
		writeError(c, http.StatusUnauthorized, "REFRESH_TOKEN_REUSED", "Refresh Token 已被使用")
	case errors.Is(err, ErrUnauthorized), errors.Is(err, ErrNotFound):
		writeError(c, http.StatusUnauthorized, "UNAUTHORIZED", "需要登录后访问")
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
