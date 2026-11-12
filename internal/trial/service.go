package trial

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

// Repository 定义游客会话和配额持久化能力
type Repository interface {
	// Create 在同一事务中创建游客工作空间和会话
	Create(ctx context.Context, session *Session, name, slug string) error
	// FindByTokenHash 按 Cookie Token 摘要查询会话
	FindByTokenHash(ctx context.Context, tokenHash []byte) (*Session, error)
	// ReserveLinkSlot 原子占用一个链接配额
	ReserveLinkSlot(ctx context.Context, sessionID string, maxLinks int, now time.Time) error
	// ReleaseLinkSlot 在链接写入失败时归还配额
	ReleaseLinkSlot(ctx context.Context, sessionID string, now time.Time) error
	// CleanupExpired 分批删除过期游客工作空间及关联数据
	CleanupExpired(ctx context.Context, now time.Time, limit int) (int, error)
	// Claim 将游客工作空间数据迁移到指定正式工作空间
	Claim(
		ctx context.Context,
		tokenHash []byte,
		targetWorkspaceID string,
		now time.Time,
	) (*ClaimResult, error)
}

// Service 创建、恢复并限制游客会话
type Service struct {
	// repo 是游客会话仓库
	repo Repository
	// sessionTTL 是游客会话和临时链接的有效期
	sessionTTL time.Duration
	// maxLinks 是单个游客会话允许创建的链接数量
	maxLinks int
	// now 提供可替换的 UTC 时间
	now func() time.Time
}

// NewService 创建游客会话服务
func NewService(repo Repository, sessionTTL time.Duration, maxLinks int) *Service {
	return &Service{
		repo:       repo,
		sessionTTL: sessionTTL,
		maxLinks:   maxLinks,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

// Create 创建游客工作空间和不透明会话 Token
func (s *Service) Create(ctx context.Context) (*Session, string, error) {
	token, tokenHash, err := generateToken()
	if err != nil {
		return nil, "", err
	}

	now := s.now()
	session := &Session{
		ID:          ulid.Make().String(),
		WorkspaceID: ulid.Make().String(),
		TokenHash:   tokenHash,
		ExpiresAt:   now.Add(s.sessionTTL),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	name := "游客试用"
	slug := "trial_" + strings.ToLower(session.ID)
	if err := s.repo.Create(ctx, session, name, slug); err != nil {
		return nil, "", err
	}
	return session, token, nil
}

// Resume 使用浏览器 Cookie 恢复未过期的游客会话
func (s *Service) Resume(ctx context.Context, token string) (*Session, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrSessionNotFound
	}
	hash := sha256.Sum256([]byte(token))
	session, err := s.repo.FindByTokenHash(ctx, hash[:])
	if err != nil {
		return nil, err
	}
	if !session.ExpiresAt.After(s.now()) {
		return nil, ErrSessionExpired
	}
	return session, nil
}

// IsInvalidSession 判断错误是否表示 Cookie 会话应被替换
func IsInvalidSession(err error) bool {
	return errors.Is(err, ErrSessionNotFound) || errors.Is(err, ErrSessionExpired)
}

// ReserveLink 为一次创建请求原子占用链接配额
func (s *Service) ReserveLink(ctx context.Context, sessionID string) error {
	return s.repo.ReserveLinkSlot(ctx, sessionID, s.maxLinks, s.now())
}

// ReleaseLink 在链接创建失败后归还已占用的配额
func (s *Service) ReleaseLink(ctx context.Context, sessionID string) error {
	return s.repo.ReleaseLinkSlot(ctx, sessionID, s.now())
}

// Remaining 返回当前会话还可以创建的链接数量
func (s *Service) Remaining(session *Session) int {
	if session.LinkCount >= uint16(s.maxLinks) {
		return 0
	}
	return s.maxLinks - int(session.LinkCount)
}

// MaxLinks 返回单个游客会话的链接上限
func (s *Service) MaxLinks() int {
	return s.maxLinks
}

// SessionTTL 返回游客 Cookie 的最大保存时长
func (s *Service) SessionTTL() time.Duration {
	return s.sessionTTL
}

// Claim 将当前 Cookie 游客会话中的链接和点击迁移到登录工作空间
func (s *Service) Claim(
	ctx context.Context,
	token, targetWorkspaceID string,
) (*ClaimResult, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrSessionNotFound
	}
	hash := sha256.Sum256([]byte(token))
	return s.repo.Claim(ctx, hash[:], targetWorkspaceID, s.now())
}

// generateToken 生成 Cookie 原始 Token 和用于数据库查询的 SHA-256 摘要
func generateToken() (string, []byte, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", nil, fmt.Errorf("生成游客 Token 失败: %w", err)
	}
	token := "trial_" + base64.RawURLEncoding.EncodeToString(random)
	hash := sha256.Sum256([]byte(token))
	return token, hash[:], nil
}
