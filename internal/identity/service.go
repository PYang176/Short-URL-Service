package identity

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/oklog/ulid/v2"
)

// Service 实现注册、登录、Refresh Token 轮换和资料管理
type Service struct {
	// repo 提供身份和工作空间持久化
	repo Repository
	// passwords 负责 Argon2id 摘要生成与校验
	passwords *PasswordHasher
	// tokens 负责 Access Token 签发与校验
	tokens *TokenManager
	// refreshTokenTTL 是新 Refresh Token 的有效期
	refreshTokenTTL time.Duration
	// now 提供可替换的 UTC 时间，便于测试轮换和过期行为
	now func() time.Time
}

// NewService 创建身份服务并注入持久化、摘要和 Token 组件
func NewService(
	repo Repository,
	passwords *PasswordHasher,
	tokens *TokenManager,
	refreshTokenTTL time.Duration,
) *Service {
	return &Service{
		repo:            repo,
		passwords:       passwords,
		tokens:          tokens,
		refreshTokenTTL: refreshTokenTTL,
		now:             func() time.Time { return time.Now().UTC() },
	}
}

// Register 创建用户、默认工作空间和首组登录凭证
func (s *Service) Register(ctx context.Context, email, password, displayName string) (*AuthResult, error) {
	// 输入边界在密码哈希之前完成，避免无效请求消耗 Argon2 成本。

	email = normalizeEmail(email)
	displayName = strings.TrimSpace(displayName)
	if err := validateRegistration(email, password, displayName); err != nil {
		return nil, err
	}

	passwordHash, err := s.passwords.Hash(password)
	if err != nil {
		return nil, fmt.Errorf("生成密码摘要失败: %w", err)
	}

	now := s.now()
	user := &User{
		ID:           newID(),
		Email:        email,
		PasswordHash: passwordHash,
		DisplayName:  displayName,
		Role:         UserRoleUser,
		Status:       UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	workspace := &Workspace{
		ID:          newID(),
		Name:        buildWorkspaceName(displayName),
		Slug:        "ws_" + strings.ToLower(user.ID),
		OwnerUserID: user.ID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// 身份写入和首个 Refresh Token 必须由仓库事务一次提交，避免留下
	// 无法登录的半成品账号。

	refreshPlain, refreshHash, err := GenerateRefreshToken()
	if err != nil {
		return nil, err
	}
	refreshTTL := s.refreshTokenTTL
	refreshToken := &RefreshToken{
		ID:        newID(),
		UserID:    user.ID,
		TokenHash: refreshHash,
		FamilyID:  newID(),
		ExpiresAt: now.Add(refreshTTL),
		CreatedAt: now,
	}
	if err := s.repo.CreateIdentity(ctx, user, workspace, refreshToken); err != nil {
		return nil, err
	}

	accessToken, accessExpires, err := s.tokens.Issue(user.ID, workspace.ID, MemberRoleOwner, now)
	if err != nil {
		return nil, err
	}
	return &AuthResult{
		User:                *user,
		Workspace:           *workspace,
		AccessToken:         accessToken,
		AccessTokenExpires:  accessExpires,
		RefreshToken:        refreshPlain,
		RefreshTokenExpires: refreshToken.ExpiresAt,
	}, nil
}

// Login 校验账号密码并为本次登录创建新的 Refresh Token Family
func (s *Service) Login(ctx context.Context, email, password string) (*AuthResult, error) {
	email = normalizeEmail(email)
	user, err := s.repo.FindUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	if user.Status != UserStatusActive {
		return nil, ErrInvalidCredentials
	}

	matches, err := s.passwords.Verify(password, user.PasswordHash)
	if err != nil {
		return nil, fmt.Errorf("校验密码失败: %w", err)
	}
	if !matches {
		return nil, ErrInvalidCredentials
	}

	workspace, err := s.repo.FindWorkspaceForUser(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	return s.newSession(ctx, user, workspace, newID())
}

// Refresh 校验并轮换 Refresh Token，旧 Token 再次出现时撤销整个 Family
func (s *Service) Refresh(ctx context.Context, refreshToken string) (*AuthResult, error) {
	// 并发边界：同一 Refresh Token 可能被并发提交。仓库使用条件更新
	// 保证只有一个请求能完成轮换，失败方按重放处理。

	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil, ErrUnauthorized
	}

	now := s.now()
	current, err := s.repo.FindRefreshTokenByHash(ctx, HashRefreshToken(refreshToken))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrUnauthorized
		}
		return nil, err
	}

	if current.RevokedAt != nil {
		if err := s.repo.RevokeTokenFamily(ctx, current.FamilyID, now); err != nil {
			return nil, err
		}
		return nil, ErrTokenReuse
	}
	if !current.ExpiresAt.After(now) {
		return nil, ErrUnauthorized
	}

	user, err := s.repo.FindUserByID(ctx, current.UserID)
	if err != nil || user.Status != UserStatusActive {
		return nil, ErrUnauthorized
	}
	workspace, err := s.repo.FindWorkspaceForUser(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	refreshPlain, refreshHash, err := GenerateRefreshToken()
	if err != nil {
		return nil, err
	}
	replacement := &RefreshToken{
		ID:        newID(),
		UserID:    user.ID,
		TokenHash: refreshHash,
		FamilyID:  current.FamilyID,
		ExpiresAt: now.Add(s.refreshTokenTTL),
		CreatedAt: now,
	}
	if err := s.repo.RotateRefreshToken(ctx, current.ID, replacement, now); err != nil {
		if errors.Is(err, ErrTokenReuse) {
			_ = s.repo.RevokeTokenFamily(ctx, current.FamilyID, now)
		}
		return nil, err
	}

	accessToken, accessExpires, err := s.tokens.Issue(user.ID, workspace.ID, MemberRoleOwner, now)
	if err != nil {
		return nil, err
	}
	return &AuthResult{
		User:                *user,
		Workspace:           *workspace,
		AccessToken:         accessToken,
		AccessTokenExpires:  accessExpires,
		RefreshToken:        refreshPlain,
		RefreshTokenExpires: replacement.ExpiresAt,
	}, nil
}

// Logout 撤销 Refresh Token 并对未知 Token 保持幂等
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	if strings.TrimSpace(refreshToken) == "" {
		return nil
	}
	return s.repo.RevokeRefreshToken(ctx, HashRefreshToken(refreshToken), s.now())
}

// Profile 返回可用用户及其默认工作空间
func (s *Service) Profile(ctx context.Context, userID string) (*User, *Workspace, error) {
	user, err := s.repo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	if user.Status != UserStatusActive {
		return nil, nil, ErrUnauthorized
	}
	workspace, err := s.repo.FindWorkspaceForUser(ctx, user.ID)
	if err != nil {
		return nil, nil, err
	}
	return user, workspace, nil
}

// UpdateDisplayName 校验并更新当前用户展示名称
func (s *Service) UpdateDisplayName(ctx context.Context, userID, displayName string) (*User, *Workspace, error) {
	displayName = strings.TrimSpace(displayName)
	if err := validateDisplayName(displayName); err != nil {
		return nil, nil, err
	}
	if err := s.repo.UpdateDisplayName(ctx, userID, displayName, s.now()); err != nil {
		return nil, nil, err
	}
	return s.Profile(ctx, userID)
}

// newSession 为已认证用户创建新的 Refresh Token 和 Access Token
func (s *Service) newSession(ctx context.Context, user *User, workspace *Workspace, familyID string) (*AuthResult, error) {
	now := s.now()
	refreshPlain, refreshHash, err := GenerateRefreshToken()
	if err != nil {
		return nil, err
	}
	refreshToken := &RefreshToken{
		ID:        newID(),
		UserID:    user.ID,
		TokenHash: refreshHash,
		FamilyID:  familyID,
		ExpiresAt: now.Add(s.refreshTokenTTL),
		CreatedAt: now,
	}
	if err := s.repo.CreateRefreshToken(ctx, refreshToken); err != nil {
		return nil, err
	}
	accessToken, accessExpires, err := s.tokens.Issue(user.ID, workspace.ID, MemberRoleOwner, now)
	if err != nil {
		return nil, err
	}
	return &AuthResult{
		User:                *user,
		Workspace:           *workspace,
		AccessToken:         accessToken,
		AccessTokenExpires:  accessExpires,
		RefreshToken:        refreshPlain,
		RefreshTokenExpires: refreshToken.ExpiresAt,
	}, nil
}

// validateRegistration 校验注册输入的最小安全边界
func validateRegistration(email, password, displayName string) error {
	if !validEmail(email) {
		return fmt.Errorf("%w: 邮箱格式错误", ErrInvalidInput)
	}
	if len(password) < 8 || len(password) > 128 {
		return fmt.Errorf("%w: 密码长度必须为 8 至 128 字节", ErrInvalidInput)
	}
	return validateDisplayName(displayName)
}

// validateDisplayName 校验展示名称的字符数范围
func validateDisplayName(displayName string) error {
	length := utf8.RuneCountInString(displayName)
	if length < 1 || length > 64 {
		return fmt.Errorf("%w: 展示名称长度必须为 1 至 64 个字符", ErrInvalidInput)
	}
	return nil
}

// validEmail 判断输入是否为不带显示名称的合法邮箱地址
func validEmail(email string) bool {
	address, err := mail.ParseAddress(email)
	if err != nil || address.Name != "" {
		return false
	}
	return strings.EqualFold(address.Address, email) && strings.Contains(email, "@")
}

// normalizeEmail 去除首尾空白并统一邮箱大小写
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// buildWorkspaceName 生成不超过数据库列长度的默认工作空间名称
func buildWorkspaceName(displayName string) string {
	const suffix = " 的空间"
	runes := []rune(displayName)
	maxNameRunes := 64 - len([]rune(suffix))
	if len(runes) > maxNameRunes {
		runes = runes[:maxNameRunes]
	}
	return string(runes) + suffix
}

// newID 生成按时间大致有序的 ULID 业务标识
func newID() string {
	return ulid.Make().String()
}
