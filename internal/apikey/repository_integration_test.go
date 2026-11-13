//go:build integration

package apikey

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/oklog/ulid/v2"
)

// TestMySQLRepositoryLifecycle 在真实 MySQL 中验证 API Key 生命周期和隔离
func TestMySQLRepositoryLifecycle(t *testing.T) {
	dsn := os.Getenv("SHORTURL_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("未配置 SHORTURL_TEST_MYSQL_DSN")
	}
	db, err := sqlx.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("打开 MySQL 失败: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("关闭 MySQL 失败: %v", err)
		}
	})
	if err := db.PingContext(t.Context()); err != nil {
		t.Fatalf("连接 MySQL 失败: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	userID := ulid.Make().String()
	workspaceID := ulid.Make().String()
	otherWorkspaceID := ulid.Make().String()
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO users (id, email, password_hash, display_name, role, status, created_at, updated_at)
		VALUES (?, ?, 'test-hash', 'API Key Test', 1, 1, ?, ?)`,
		userID, "apikey-"+userID+"@example.com", now, now,
	); err != nil {
		t.Fatalf("创建测试用户失败: %v", err)
	}
	for index, id := range []string{workspaceID, otherWorkspaceID} {
		if _, err := db.ExecContext(t.Context(), `
			INSERT INTO workspaces (id, kind, name, slug, owner_user_id, created_at, updated_at)
			VALUES (?, 1, ?, ?, ?, ?, ?)`,
			id, "API Key "+id, "apikey_"+id, userID, now, now,
		); err != nil {
			t.Fatalf("创建工作空间 %d 失败: %v", index+1, err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM api_keys WHERE workspace_id IN (?, ?)`, workspaceID, otherWorkspaceID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM workspaces WHERE id IN (?, ?)`, workspaceID, otherWorkspaceID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id = ?`, userID)
	})

	repo := NewMySQLRepository(db)
	service := NewService(repo)
	created, err := service.Create(
		t.Context(),
		workspaceID,
		userID,
		"integration",
		[]string{ScopeLinksRead, ScopeAnalyticsRead},
		nil,
	)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	principal, err := service.Authenticate(t.Context(), created.Plaintext)
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if principal.WorkspaceID != workspaceID {
		t.Fatalf("WorkspaceID = %q, want %q", principal.WorkspaceID, workspaceID)
	}
	items, err := repo.ListByWorkspace(t.Context(), workspaceID)
	if err != nil || len(items) != 1 {
		t.Fatalf("ListByWorkspace() = %+v, %v", items, err)
	}
	if items[0].LastUsedAt == nil {
		t.Fatal("LastUsedAt must be updated after authentication")
	}
	if err := service.Revoke(t.Context(), otherWorkspaceID, created.Key.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-workspace Revoke() error = %v", err)
	}
	if err := service.Revoke(t.Context(), workspaceID, created.Key.ID); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	if _, err := service.Authenticate(t.Context(), created.Plaintext); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("revoked Authenticate() error = %v", err)
	}
}
