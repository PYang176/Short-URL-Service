package apikey

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestServiceCreateAuthenticateAndRevoke 验证 API Key 完整生命周期
func TestServiceCreateAuthenticateAndRevoke(t *testing.T) {
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	repo := newFakeRepository()
	service := NewService(repo)
	service.now = func() time.Time { return now }

	created, err := service.Create(
		context.Background(),
		"workspace-1",
		"user-1",
		"演示脚本",
		[]string{ScopeAnalyticsRead, ScopeLinksRead, ScopeLinksRead},
		nil,
	)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !strings.HasPrefix(created.Plaintext, keySchemePrefix) {
		t.Fatalf("plaintext = %q", created.Plaintext)
	}
	if len(created.Key.KeyPrefix) != prefixLength {
		t.Fatalf("prefix length = %d, want %d", len(created.Key.KeyPrefix), prefixLength)
	}
	if created.Key.Scopes != ScopeLinksRead+","+ScopeAnalyticsRead {
		t.Fatalf("scopes = %q", created.Key.Scopes)
	}
	if len(created.Key.SecretHash) != 32 || string(created.Key.SecretHash) == created.Plaintext {
		t.Fatal("API Key secret must only be stored as SHA-256 hash")
	}

	principal, err := service.Authenticate(context.Background(), created.Plaintext)
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if principal.ID != created.Key.ID || principal.WorkspaceID != "workspace-1" {
		t.Fatalf("principal = %+v", principal)
	}
	if len(principal.Scopes) != 2 {
		t.Fatalf("principal scopes = %+v", principal.Scopes)
	}

	tampered := created.Plaintext[:len(created.Plaintext)-1] + "x"
	if _, err := service.Authenticate(context.Background(), tampered); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("tampered Authenticate() error = %v", err)
	}

	if err := service.Revoke(context.Background(), "workspace-1", created.Key.ID); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	if _, err := service.Authenticate(context.Background(), created.Plaintext); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("revoked Authenticate() error = %v", err)
	}
}

// TestServiceRejectsInvalidScopes 验证未知或空作用域不能创建 Key
func TestServiceRejectsInvalidScopes(t *testing.T) {
	tests := []struct {
		name   string
		scopes []string
	}{
		{name: "empty", scopes: nil},
		{name: "unknown", scopes: []string{"admin:all"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := NewService(newFakeRepository())
			_, err := service.Create(
				context.Background(),
				"workspace-1",
				"user-1",
				"invalid",
				test.scopes,
				nil,
			)
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("Create() error = %v, want ErrInvalidInput", err)
			}
		})
	}
}

// TestServiceRejectsExpiredKey 验证过期 Key 无法通过认证
func TestServiceRejectsExpiredKey(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(repo)
	created, err := service.Create(
		context.Background(),
		"workspace-1",
		"user-1",
		"expired",
		[]string{ScopeLinksRead},
		nil,
	)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	expiredAt := service.now().Add(-time.Minute)
	repo.keys[created.Key.ID].ExpiresAt = &expiredAt
	if _, err := service.Authenticate(context.Background(), created.Plaintext); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("Authenticate() error = %v, want ErrUnauthorized", err)
	}
}

type fakeRepository struct {
	keys map[string]*Key
}

// newFakeRepository 创建 API Key 内存仓库
func newFakeRepository() *fakeRepository {
	return &fakeRepository{keys: make(map[string]*Key)}
}

// Create 保存内存 API Key
func (r *fakeRepository) Create(_ context.Context, key *Key) error {
	copy := cloneKey(key)
	r.keys[key.ID] = copy
	return nil
}

// ListByWorkspace 返回内存工作空间 Key
func (r *fakeRepository) ListByWorkspace(_ context.Context, workspaceID string) ([]Key, error) {
	items := make([]Key, 0)
	for _, key := range r.keys {
		if key.WorkspaceID == workspaceID {
			items = append(items, *cloneKey(key))
		}
	}
	return items, nil
}

// FindByPrefix 按内存前缀查询 Key
func (r *fakeRepository) FindByPrefix(_ context.Context, prefix string) (*Key, error) {
	for _, key := range r.keys {
		if key.KeyPrefix == prefix {
			return cloneKey(key), nil
		}
	}
	return nil, ErrNotFound
}

// Revoke 撤销内存 Key
func (r *fakeRepository) Revoke(_ context.Context, workspaceID, id string, now time.Time) error {
	key, ok := r.keys[id]
	if !ok || key.WorkspaceID != workspaceID {
		return ErrNotFound
	}
	if key.RevokedAt == nil {
		value := now
		key.RevokedAt = &value
	}
	return nil
}

// TouchLastUsed 更新内存 Key 最近使用时间
func (r *fakeRepository) TouchLastUsed(_ context.Context, id string, now time.Time) error {
	if key, ok := r.keys[id]; ok {
		value := now
		key.LastUsedAt = &value
	}
	return nil
}

// cloneKey 深复制 API Key 可空时间和哈希
func cloneKey(key *Key) *Key {
	copy := *key
	copy.SecretHash = append([]byte(nil), key.SecretHash...)
	if key.LastUsedAt != nil {
		value := *key.LastUsedAt
		copy.LastUsedAt = &value
	}
	if key.ExpiresAt != nil {
		value := *key.ExpiresAt
		copy.ExpiresAt = &value
	}
	if key.RevokedAt != nil {
		value := *key.RevokedAt
		copy.RevokedAt = &value
	}
	return &copy
}
