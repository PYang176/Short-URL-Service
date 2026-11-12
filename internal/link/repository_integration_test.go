//go:build integration

package link

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

// TestMySQLRepositoryLifecycle 在真实 MySQL 中验证域名、链接 CRUD 和软删除
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
	email := "integration-" + userID + "@example.com"

	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO users (id, email, password_hash, display_name, role, status, created_at, updated_at)
		VALUES (?, ?, 'test-hash', 'Integration User', 1, 1, ?, ?)`,
		userID, email, now, now,
	); err != nil {
		t.Fatalf("创建测试用户失败: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO workspaces (id, name, slug, owner_user_id, created_at, updated_at)
		VALUES (?, 'Integration Workspace', ?, ?, ?, ?)`,
		workspaceID, "ws-"+workspaceID, userID, now, now,
	); err != nil {
		t.Fatalf("创建测试工作空间失败: %v", err)
	}

	repo := NewMySQLRepository(db)
	domain, err := repo.EnsureDefaultDomain(t.Context(), "integration.local", now)
	if err != nil {
		t.Fatalf("EnsureDefaultDomain() error = %v", err)
	}

	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM links WHERE workspace_id = ?`, workspaceID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM domains WHERE id = ?`, domain.ID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM workspaces WHERE id = ?`, workspaceID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id = ?`, userID)
	})

	destination := "https://example.com/integration"
	destinationHash := sha256.Sum256([]byte(destination))
	link := &Link{
		ID:              ulid.Make().String(),
		WorkspaceID:     workspaceID,
		DomainID:        domain.ID,
		Slug:            "integration",
		DestinationURL:  destination,
		DestinationHash: destinationHash[:],
		Title:           "Integration",
		Notes:           "",
		Status:          StatusActive,
		RedirectCode:    302,
		Version:         1,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := repo.Create(t.Context(), link); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := repo.Create(t.Context(), cloneLink(link)); !errors.Is(err, ErrSlugTaken) {
		t.Fatalf("重复短码 Create() error = %v, want ErrSlugTaken", err)
	}

	found, err := repo.FindByDomainSlug(t.Context(), domain.ID, link.Slug)
	if err != nil {
		t.Fatalf("FindByDomainSlug() error = %v", err)
	}
	if found.DestinationURL != destination {
		t.Fatalf("目标地址 = %q, want %q", found.DestinationURL, destination)
	}

	links, err := repo.List(t.Context(), workspaceID, 20, nil, "")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("List() count = %d, want 1", len(links))
	}

	found.DestinationURL = "https://example.com/changed"
	changedHash := sha256.Sum256([]byte(found.DestinationURL))
	found.DestinationHash = changedHash[:]
	found.Version++
	found.UpdatedAt = now.Add(time.Second)
	if err := repo.Update(t.Context(), found); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	updated, err := repo.FindByID(t.Context(), workspaceID, link.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if updated.DestinationURL != "https://example.com/changed" {
		t.Fatalf("更新后目标地址 = %q", updated.DestinationURL)
	}

	if err := repo.SoftDelete(t.Context(), workspaceID, link.ID, now.Add(2*time.Second)); err != nil {
		t.Fatalf("SoftDelete() error = %v", err)
	}
	if _, err := repo.FindByID(t.Context(), workspaceID, link.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("删除后 FindByID() error = %v, want ErrNotFound", err)
	}
}
