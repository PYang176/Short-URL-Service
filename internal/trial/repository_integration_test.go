//go:build integration

package trial

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

// TestMySQLRepositoryLifecycle 在真实 MySQL 中验证游客会话、配额和清理
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
	session := &Session{
		ID:          ulid.Make().String(),
		WorkspaceID: ulid.Make().String(),
		TokenHash:   []byte("01234567890123456789012345678901"),
		ExpiresAt:   now.Add(time.Hour),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	repo := NewMySQLRepository(db)
	if err := repo.Create(t.Context(), session, "游客集成测试", "trial_"+session.ID); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(
			context.Background(),
			`DELETE FROM trial_sessions WHERE workspace_id = ?`,
			session.WorkspaceID,
		)
		_, _ = db.ExecContext(
			context.Background(),
			`DELETE FROM workspaces WHERE id = ?`,
			session.WorkspaceID,
		)
	})

	found, err := repo.FindByTokenHash(t.Context(), session.TokenHash)
	if err != nil {
		t.Fatalf("FindByTokenHash() error = %v", err)
	}
	if found.WorkspaceID != session.WorkspaceID {
		t.Fatalf("WorkspaceID = %q, want %q", found.WorkspaceID, session.WorkspaceID)
	}

	for index := 0; index < 3; index++ {
		if err := repo.ReserveLinkSlot(t.Context(), session.ID, 3, now); err != nil {
			t.Fatalf("ReserveLinkSlot() attempt %d error = %v", index+1, err)
		}
	}
	if err := repo.ReserveLinkSlot(t.Context(), session.ID, 3, now); !errors.Is(err, ErrLinkLimitReached) {
		t.Fatalf("ReserveLinkSlot() over limit error = %v", err)
	}
	if err := repo.ReleaseLinkSlot(t.Context(), session.ID, now); err != nil {
		t.Fatalf("ReleaseLinkSlot() error = %v", err)
	}

	if _, err := db.ExecContext(
		t.Context(),
		`UPDATE trial_sessions SET expires_at = ? WHERE id = ?`,
		now.Add(-time.Minute),
		session.ID,
	); err != nil {
		t.Fatalf("使游客会话过期失败: %v", err)
	}
	deleted, err := repo.CleanupExpired(t.Context(), now, 100)
	if err != nil {
		t.Fatalf("CleanupExpired() error = %v", err)
	}
	if deleted != 1 {
		t.Fatalf("CleanupExpired() deleted = %d, want 1", deleted)
	}
}
