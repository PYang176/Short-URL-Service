package identity

import (
	"errors"
	"testing"
	"time"
)

// TestTokenManagerIssueAndParse 验证 Access Token 声明和过期时间
func TestTokenManagerIssueAndParse(t *testing.T) {
	now := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	manager := NewTokenManager("0123456789abcdef0123456789abcdef", 15*time.Minute)

	token, expiresAt, err := manager.Issue("user-1", "workspace-1", MemberRoleOwner, now)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if want := now.Add(15 * time.Minute); !expiresAt.Equal(want) {
		t.Fatalf("expiresAt = %v, want %v", expiresAt, want)
	}

	claims, err := manager.Parse(token, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if claims.Subject != "user-1" || claims.WorkspaceID != "workspace-1" {
		t.Fatalf("claims = %+v, want user and workspace IDs", claims)
	}
}

// TestTokenManagerRejectsExpiredToken 验证过期 Access Token 被拒绝
func TestTokenManagerRejectsExpiredToken(t *testing.T) {
	now := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	manager := NewTokenManager("0123456789abcdef0123456789abcdef", time.Minute)
	token, _, err := manager.Issue("user-1", "workspace-1", MemberRoleOwner, now)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	_, err = manager.Parse(token, now.Add(2*time.Minute))
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("Parse() error = %v, want ErrUnauthorized", err)
	}
}
