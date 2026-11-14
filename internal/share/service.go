package share

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

const shareTokenPrefix = "shr_"

// CreateResult 是创建分享后的公开 Token 和分享元数据
type CreateResult struct {
	// Share 是持久化后的分享记录
	Share Share
	// Token 是只在创建响应中返回的公开分享 Token
	Token string
}

// Service 管理公开分析分享生命周期
type Service struct {
	// repo 是分享持久化仓库
	repo Repository
	// now 提供可替换的 UTC 时间
	now func() time.Time
}

// NewService 创建公开分析分享服务
func NewService(repo Repository) *Service {
	return &Service{
		repo: repo,
		now:  func() time.Time { return time.Now().UTC() },
	}
}

// Create 创建指定分析范围和有效期的公开分享
func (s *Service) Create(
	ctx context.Context,
	workspaceID, userID, linkID string,
	days int,
) (*CreateResult, error) {
	if days != 1 && days != 7 && days != 30 {
		return nil, fmt.Errorf("%w: 分享范围只能为 1、7 或 30 天", ErrInvalidInput)
	}
	token, tokenHash, err := generateToken()
	if err != nil {
		return nil, err
	}
	now := s.now()
	item := &Share{
		ID:              ulid.Make().String(),
		WorkspaceID:     workspaceID,
		LinkID:          linkID,
		CreatedByUserID: userID,
		TokenHash:       tokenHash,
		Days:            days,
		ExpiresAt:       now.Add(time.Duration(days) * 24 * time.Hour),
		CreatedAt:       now,
	}
	if err := s.repo.Create(ctx, item); err != nil {
		return nil, err
	}
	return &CreateResult{Share: *item, Token: token}, nil
}

// List 返回短链当前有效的分享记录
func (s *Service) List(
	ctx context.Context,
	workspaceID, linkID string,
) ([]Share, error) {
	return s.repo.ListActive(ctx, workspaceID, linkID, s.now())
}

// Revoke 撤销指定分享
func (s *Service) Revoke(
	ctx context.Context,
	workspaceID, linkID, id string,
) error {
	return s.repo.Revoke(ctx, workspaceID, linkID, id, s.now())
}

// Resolve 使用公开 Token 查询有效分享
func (s *Service) Resolve(ctx context.Context, token string) (*Share, error) {
	token = strings.TrimSpace(token)
	if !strings.HasPrefix(token, shareTokenPrefix) {
		return nil, ErrNotFound
	}
	return s.repo.FindActiveByTokenHash(ctx, HashToken(token), s.now())
}

// HashToken 计算公开分享 Token 的 SHA-256 摘要
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return sum[:]
}

// generateToken 生成不可猜测的公开分享 Token 和存储摘要
func generateToken() (string, []byte, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", nil, fmt.Errorf("生成公开分享 Token 失败: %w", err)
	}
	token := shareTokenPrefix + base64.RawURLEncoding.EncodeToString(random)
	return token, HashToken(token), nil
}
