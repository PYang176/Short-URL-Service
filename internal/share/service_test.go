package share

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeRepository struct {
	item *Share
}

// Create 保存测试分享记录
func (r *fakeRepository) Create(_ context.Context, item *Share) error {
	copied := *item
	r.item = &copied
	return nil
}

// ListActive 返回尚未撤销的测试分享
func (r *fakeRepository) ListActive(context.Context, string, string, time.Time) ([]Share, error) {
	if r.item == nil || r.item.RevokedAt != nil {
		return []Share{}, nil
	}
	return []Share{*r.item}, nil
}

// Revoke 撤销测试分享
func (r *fakeRepository) Revoke(_ context.Context, _, _, _ string, now time.Time) error {
	if r.item == nil {
		return ErrNotFound
	}
	r.item.RevokedAt = &now
	return nil
}

// FindActiveByTokenHash 按摘要返回测试分享
func (r *fakeRepository) FindActiveByTokenHash(
	_ context.Context,
	hash []byte,
	now time.Time,
) (*Share, error) {
	if r.item == nil || r.item.RevokedAt != nil ||
		!r.item.ExpiresAt.After(now) || !bytes.Equal(r.item.TokenHash, hash) {
		return nil, ErrNotFound
	}
	copied := *r.item
	return &copied, nil
}

// TestServiceLifecycle 验证分享 Token、有效期和撤销流程
func TestServiceLifecycle(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)
	now := time.Date(2026, 11, 15, 8, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	result, err := service.Create(t.Context(), "workspace-1", "user-1", "link-1", 7)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !strings.HasPrefix(result.Token, shareTokenPrefix) || result.Token == "" {
		t.Fatalf("Token = %q", result.Token)
	}
	if !bytes.Equal(result.Share.TokenHash, HashToken(result.Token)) {
		t.Fatal("Token 摘要不匹配")
	}
	if !result.Share.ExpiresAt.Equal(now.Add(7 * 24 * time.Hour)) {
		t.Fatalf("ExpiresAt = %v", result.Share.ExpiresAt)
	}

	resolved, err := service.Resolve(t.Context(), result.Token)
	if err != nil || resolved.ID != result.Share.ID {
		t.Fatalf("Resolve() = %+v, %v", resolved, err)
	}
	items, err := service.List(t.Context(), "workspace-1", "link-1")
	if err != nil || len(items) != 1 {
		t.Fatalf("List() = %+v, %v", items, err)
	}
	if err := service.Revoke(t.Context(), "workspace-1", "link-1", result.Share.ID); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	if _, err := service.Resolve(t.Context(), result.Token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("撤销后 Resolve() error = %v", err)
	}
}

// TestServiceRejectsInvalidDays 验证公开分享只允许固定分析范围
func TestServiceRejectsInvalidDays(t *testing.T) {
	service := NewService(&fakeRepository{})
	if _, err := service.Create(t.Context(), "workspace-1", "user-1", "link-1", 14); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Create() error = %v, want ErrInvalidInput", err)
	}
}
