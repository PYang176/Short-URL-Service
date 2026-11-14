//go:build integration

package share

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

	"github.com/PYang176/Short-URL-Service/internal/link"
)

// TestMySQLRepositoryLifecycle 在真实 MySQL 中验证分享创建、查询和撤销
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
	linkID := ulid.Make().String()
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO users (id, email, password_hash, display_name, role, status, created_at, updated_at)
		VALUES (?, ?, 'test-hash', 'Share Integration', 1, 1, ?, ?)`,
		userID, "share-"+userID+"@example.com", now, now,
	); err != nil {
		t.Fatalf("创建测试用户失败: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO workspaces (id, kind, name, slug, owner_user_id, created_at, updated_at)
		VALUES (?, 1, 'Share Integration', ?, ?, ?, ?)`,
		workspaceID, "share-"+workspaceID, userID, now, now,
	); err != nil {
		t.Fatalf("创建测试工作空间失败: %v", err)
	}
	linkRepo := link.NewMySQLRepository(db)
	domain, err := linkRepo.EnsureDefaultDomain(t.Context(), "share-"+workspaceID+".test", now)
	if err != nil {
		t.Fatalf("创建测试域名失败: %v", err)
	}
	destinationHash := sha256.Sum256([]byte("https://example.com/share"))
	if err := linkRepo.Create(t.Context(), &link.Link{
		ID:              linkID,
		WorkspaceID:     workspaceID,
		DomainID:        domain.ID,
		DomainHostname:  domain.Hostname,
		Slug:            "share-report",
		DestinationURL:  "https://example.com/share",
		DestinationHash: destinationHash[:],
		Status:          link.StatusActive,
		RedirectCode:    302,
		Version:         1,
		CreatedAt:       now,
		UpdatedAt:       now,
	}, nil); err != nil {
		t.Fatalf("创建测试链接失败: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM analytics_shares WHERE workspace_id = ?`, workspaceID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM links WHERE workspace_id = ?`, workspaceID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM domains WHERE id = ?`, domain.ID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM workspaces WHERE id = ?`, workspaceID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id = ?`, userID)
	})

	repo := NewMySQLRepository(db)
	token := "shr_integration_token"
	item := &Share{
		ID:              ulid.Make().String(),
		WorkspaceID:     workspaceID,
		LinkID:          linkID,
		CreatedByUserID: userID,
		TokenHash:       HashToken(token),
		Days:            7,
		ExpiresAt:       now.Add(7 * 24 * time.Hour),
		CreatedAt:       now,
	}
	if err := repo.Create(t.Context(), item); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	found, err := repo.FindActiveByTokenHash(t.Context(), HashToken(token), now)
	if err != nil || found.ID != item.ID {
		t.Fatalf("FindActiveByTokenHash() = %+v, %v", found, err)
	}
	items, err := repo.ListActive(t.Context(), workspaceID, linkID, now)
	if err != nil || len(items) != 1 {
		t.Fatalf("ListActive() = %+v, %v", items, err)
	}
	if err := repo.Revoke(t.Context(), workspaceID, linkID, item.ID, now.Add(time.Minute)); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	if _, err := repo.FindActiveByTokenHash(
		t.Context(),
		HashToken(token),
		now.Add(2*time.Minute),
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("撤销后 FindActiveByTokenHash() error = %v", err)
	}
}
