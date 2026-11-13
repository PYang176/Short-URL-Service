package apikey

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

const (
	// keySchemePrefix 是所有 API Key 的固定协议前缀
	keySchemePrefix = "sl_live_"
	// prefixLength 是存储和查询使用的随机前缀长度
	prefixLength = 12
)

// Repository 定义 API Key 生命周期持久化能力
type Repository interface {
	// Create 保存新的 API Key 摘要
	Create(ctx context.Context, key *Key) error
	// ListByWorkspace 返回工作空间 API Key 元数据
	ListByWorkspace(ctx context.Context, workspaceID string) ([]Key, error)
	// FindByPrefix 按公开前缀查询候选 Key
	FindByPrefix(ctx context.Context, prefix string) (*Key, error)
	// Revoke 撤销当前工作空间的 API Key
	Revoke(ctx context.Context, workspaceID, id string, now time.Time) error
	// TouchLastUsed 限频更新最近认证时间
	TouchLastUsed(ctx context.Context, id string, now time.Time) error
}

// Service 创建、认证和撤销 API Key
type Service struct {
	// repo 是 API Key 仓库
	repo Repository
	// now 提供可替换的 UTC 时间
	now func() time.Time
}

// NewService 创建 API Key 服务
func NewService(repo Repository) *Service {
	return &Service{
		repo: repo,
		now:  func() time.Time { return time.Now().UTC() },
	}
}

// Create 生成只展示一次的新 API Key
func (s *Service) Create(
	ctx context.Context,
	workspaceID, userID, name string,
	requestedScopes []string,
	expiresAt *time.Time,
) (*CreateResult, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 64 {
		return nil, fmt.Errorf("%w: 名称长度必须为 1 至 64 个字符", ErrInvalidInput)
	}
	scopes, err := normalizeScopes(requestedScopes)
	if err != nil {
		return nil, err
	}
	now := s.now()
	if expiresAt != nil {
		value := expiresAt.UTC()
		if !value.After(now) {
			return nil, fmt.Errorf("%w: 过期时间必须晚于当前时间", ErrInvalidInput)
		}
		expiresAt = &value
	}

	for attempt := 0; attempt < 4; attempt++ {
		prefix, raw, err := generateKey()
		if err != nil {
			return nil, err
		}
		hash := sha256.Sum256([]byte(raw))
		key := &Key{
			ID:              ulid.Make().String(),
			WorkspaceID:     workspaceID,
			CreatedByUserID: userID,
			Name:            name,
			KeyPrefix:       prefix,
			SecretHash:      hash[:],
			Scopes:          strings.Join(scopes, ","),
			ExpiresAt:       expiresAt,
			CreatedAt:       now,
		}
		if err := s.repo.Create(ctx, key); err == nil {
			return &CreateResult{Key: *key, Plaintext: raw}, nil
		} else if !errors.Is(err, ErrPrefixConflict) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("生成唯一 API Key 前缀失败")
}

// List 返回工作空间 API Key 元数据
func (s *Service) List(ctx context.Context, workspaceID string) ([]Key, error) {
	return s.repo.ListByWorkspace(ctx, workspaceID)
}

// Revoke 撤销当前工作空间的 API Key
func (s *Service) Revoke(ctx context.Context, workspaceID, id string) error {
	return s.repo.Revoke(ctx, workspaceID, id, s.now())
}

// Authenticate 校验原始 API Key 并返回工作空间作用域
func (s *Service) Authenticate(ctx context.Context, raw string) (*Principal, error) {
	prefix, err := parsePrefix(raw)
	if err != nil {
		return nil, err
	}
	key, err := s.repo.FindByPrefix(ctx, prefix)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrUnauthorized
		}
		return nil, err
	}
	hash := sha256.Sum256([]byte(raw))
	if subtle.ConstantTimeCompare(hash[:], key.SecretHash) != 1 {
		return nil, ErrUnauthorized
	}
	now := s.now()
	if key.RevokedAt != nil || (key.ExpiresAt != nil && !key.ExpiresAt.After(now)) {
		return nil, ErrUnauthorized
	}
	scopes := splitScopes(key.Scopes)
	if len(scopes) == 0 {
		return nil, ErrUnauthorized
	}
	if err := s.repo.TouchLastUsed(ctx, key.ID, now); err != nil {
		// 最近使用时间是审计辅助数据，写入失败不能中断已经完成的安全认证。
		slog.Warn("更新 API Key 最近使用时间失败", "api_key_id", key.ID, "error", err)
	}
	return &Principal{
		ID:          key.ID,
		WorkspaceID: key.WorkspaceID,
		Scopes:      scopes,
	}, nil
}

// normalizeScopes 去重并校验请求作用域
func normalizeScopes(requested []string) ([]string, error) {
	if len(requested) == 0 {
		return nil, fmt.Errorf("%w: 至少选择一个作用域", ErrInvalidInput)
	}
	requestedSet := make(map[string]struct{}, len(requested))
	for _, scope := range requested {
		requestedSet[strings.TrimSpace(scope)] = struct{}{}
	}
	result := make([]string, 0, len(requestedSet))
	for _, allowed := range AllScopes {
		if _, ok := requestedSet[allowed]; ok {
			result = append(result, allowed)
		}
	}
	if len(result) != len(requestedSet) {
		return nil, fmt.Errorf("%w: 包含不支持的作用域", ErrInvalidInput)
	}
	return result, nil
}

// splitScopes 将数据库作用域字符串转换为切片
func splitScopes(raw string) []string {
	parts := strings.Split(strings.TrimSpace(raw), ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if scope := strings.TrimSpace(part); scope != "" {
			result = append(result, scope)
		}
	}
	return result
}

// generateKey 生成前缀、Secret 和完整原始 Key
func generateKey() (string, string, error) {
	prefixBytes := make([]byte, 9)
	if _, err := rand.Read(prefixBytes); err != nil {
		return "", "", fmt.Errorf("生成 API Key 前缀失败: %w", err)
	}
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		return "", "", fmt.Errorf("生成 API Key Secret 失败: %w", err)
	}
	prefix := base64.RawURLEncoding.EncodeToString(prefixBytes)
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)
	return prefix, keySchemePrefix + prefix + "." + secret, nil
}

// parsePrefix 从原始 API Key 中提取固定前缀
func parsePrefix(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, keySchemePrefix) {
		return "", ErrUnauthorized
	}
	remaining := strings.TrimPrefix(raw, keySchemePrefix)
	parts := strings.SplitN(remaining, ".", 2)
	if len(parts) != 2 || len(parts[0]) != prefixLength || len(parts[1]) < 32 {
		return "", ErrUnauthorized
	}
	return parts[0], nil
}
