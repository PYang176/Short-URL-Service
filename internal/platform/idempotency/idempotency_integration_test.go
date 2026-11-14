//go:build integration

package idempotency

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/oklog/ulid/v2"
)

// TestMySQLRepositoryLifecycle 在真实 MySQL 中验证领取、完成和冲突
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
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO users (id, email, password_hash, display_name, role, status, created_at, updated_at)
		VALUES (?, ?, 'test-hash', 'Idempotency Test', 1, 1, ?, ?)`,
		userID, "idem-"+userID+"@example.com", now, now,
	); err != nil {
		t.Fatalf("创建测试用户失败: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO workspaces (id, name, slug, owner_user_id, created_at, updated_at)
		VALUES (?, 'Idempotency', ?, ?, ?, ?)`,
		workspaceID, "idem-"+workspaceID, userID, now, now,
	); err != nil {
		t.Fatalf("创建测试工作空间失败: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM idempotency_keys WHERE workspace_id = ?`, workspaceID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM workspace_members WHERE workspace_id = ?`, workspaceID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM workspaces WHERE id = ?`, workspaceID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id = ?`, userID)
	})

	repo := NewMySQLRepository(db)
	service := NewService(repo, time.Hour)
	service.now = func() time.Time { return now }
	key := "request-key-1"
	hash := sha256.Sum256([]byte("request-body"))

	record, claimed, err := service.Begin(t.Context(), workspaceID, key, hash[:])
	if err != nil || !claimed || record.State != StateProcessing {
		t.Fatalf("Begin() = %+v, %v, %v", record, claimed, err)
	}
	if err := service.Complete(t.Context(), workspaceID, key, hash[:], 201, `{"id":"1"}`); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	record, claimed, err = service.Begin(t.Context(), workspaceID, key, hash[:])
	if err != nil || claimed || record.State != StateCompleted {
		t.Fatalf("Begin() replay = %+v, %v, %v", record, claimed, err)
	}
	otherHash := sha256.Sum256([]byte("other-body"))
	if _, _, err := service.Begin(t.Context(), workspaceID, key, otherHash[:]); !errors.Is(err, ErrRequestMismatch) {
		t.Fatalf("Begin() mismatch error = %v", err)
	}

	leaseKey := "request-key-lease"
	leaseHash := sha256.Sum256([]byte("lease-body"))
	record, claimed, err = service.Begin(t.Context(), workspaceID, leaseKey, leaseHash[:])
	if err != nil || !claimed || record.State != StateProcessing {
		t.Fatalf("Begin() lease = %+v, %v, %v", record, claimed, err)
	}
	service.now = func() time.Time { return now.Add(31 * time.Second) }
	record, claimed, err = service.Begin(t.Context(), workspaceID, leaseKey, leaseHash[:])
	if err != nil || !claimed || record.State != StateProcessing {
		t.Fatalf("Begin() reclaim = %+v, %v, %v", record, claimed, err)
	}
}
