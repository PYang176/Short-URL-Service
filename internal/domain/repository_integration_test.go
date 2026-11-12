//go:build integration

package domain

import (
	"context"
	"os"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/oklog/ulid/v2"
)

// TestMySQLRepositoryLifecycle 在真实 MySQL 中验证域名添加、验证和删除
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
	email := "domain-integration-" + userID + "@example.com"
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO users (id, email, password_hash, display_name, role, status, created_at, updated_at)
		VALUES (?, ?, 'test-hash', 'Domain Integration', 1, 1, ?, ?)`,
		userID, email, now, now,
	); err != nil {
		t.Fatalf("创建测试用户失败: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO workspaces (id, name, slug, owner_user_id, created_at, updated_at)
		VALUES (?, 'Domain Integration', ?, ?, ?, ?)`,
		workspaceID, "domain-"+workspaceID, userID, now, now,
	); err != nil {
		t.Fatalf("创建测试工作空间失败: %v", err)
	}

	hostname := "go-" + ulid.Make().String() + ".example.com"
	repo := NewMySQLRepository(db)
	service := NewService(repo, DNSResolver{}, verifyModeMock, "links.localhost")
	service.now = func() time.Time { return now }

	added, err := service.Add(t.Context(), workspaceID, hostname)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM links WHERE domain_id = ?`, added.ID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM domains WHERE id = ?`, added.ID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM workspace_members WHERE workspace_id = ?`, workspaceID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM workspaces WHERE id = ?`, workspaceID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id = ?`, userID)
	})

	verified, err := service.Verify(t.Context(), workspaceID, added.ID)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if verified.Status != StatusVerified {
		t.Fatalf("Verify() status = %d", verified.Status)
	}
	resolvedID, err := service.ResolveVerifiedHost(t.Context(), hostname)
	if err != nil || resolvedID != added.ID {
		t.Fatalf("ResolveVerifiedHost() = %q, %v", resolvedID, err)
	}
	if err := service.Delete(t.Context(), workspaceID, added.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
}
