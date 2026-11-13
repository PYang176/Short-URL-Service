//go:build integration

package trial

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/oklog/ulid/v2"

	"github.com/PYang176/Short-URL-Service/internal/analytics"
	"github.com/PYang176/Short-URL-Service/internal/link"
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

// TestMySQLRepositoryClaim 验证链接、点击和游客数据在同一事务中完成认领
func TestMySQLRepositoryClaim(t *testing.T) {
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
	targetWorkspaceID := ulid.Make().String()
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO users (id, email, password_hash, display_name, role, status, created_at, updated_at)
		VALUES (?, ?, 'test-hash', 'Claim Test', 1, 1, ?, ?)`,
		userID, "claim-"+userID+"@example.com", now, now,
	); err != nil {
		t.Fatalf("创建目标用户失败: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO workspaces (id, kind, name, slug, owner_user_id, created_at, updated_at)
		VALUES (?, 1, 'Claim Target', ?, ?, ?, ?)`,
		targetWorkspaceID, "claim_"+targetWorkspaceID, userID, now, now,
	); err != nil {
		t.Fatalf("创建目标工作空间失败: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM click_events WHERE workspace_id = ?`, targetWorkspaceID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM links WHERE workspace_id = ?`, targetWorkspaceID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM workspaces WHERE id = ?`, targetWorkspaceID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id = ?`, userID)
	})

	repo := NewMySQLRepository(db)
	session := &Session{
		ID:          ulid.Make().String(),
		WorkspaceID: ulid.Make().String(),
		TokenHash:   []byte("abcdefghijklmnopqrstuvwxyz012345"),
		ExpiresAt:   now.Add(time.Hour),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := repo.Create(t.Context(), session, "游客认领测试", "trial_claim_"+session.ID); err != nil {
		t.Fatalf("创建游客会话失败: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM click_events WHERE workspace_id = ?`, session.WorkspaceID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM links WHERE workspace_id = ?`, session.WorkspaceID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM trial_sessions WHERE workspace_id = ?`, session.WorkspaceID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM workspaces WHERE id = ?`, session.WorkspaceID)
	})

	domains := link.NewMySQLRepository(db)
	defaultDomain, err := domains.EnsureDefaultDomain(t.Context(), "claim.test", now)
	if err != nil {
		t.Fatalf("创建默认域名失败: %v", err)
	}
	linkID := ulid.Make().String()
	expiresAt := session.ExpiresAt
	if err := domains.Create(t.Context(), &link.Link{
		ID:              linkID,
		WorkspaceID:     session.WorkspaceID,
		DomainID:        defaultDomain.ID,
		DomainHostname:  defaultDomain.Hostname,
		Slug:            "claim_" + linkID,
		DestinationURL:  "https://example.com/claim",
		DestinationHash: make([]byte, 32),
		Status:          link.StatusActive,
		RedirectCode:    302,
		ExpiresAt:       &expiresAt,
		Version:         1,
		CreatedAt:       now,
		UpdatedAt:       now,
	}, nil); err != nil {
		t.Fatalf("创建游客链接失败: %v", err)
	}
	eventID := ulid.Make().String()
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO click_events (
			event_id, workspace_id, link_id, domain_id, slug, clicked_at,
			visitor_hash, country, device, os, browser, language, referrer,
			utm_source, utm_medium, utm_campaign, utm_term, utm_content, is_bot
		) VALUES (?, ?, ?, ?, ?, ?, ?, '', '', '', '', '', '', '', '', '', '', '', 0)`,
		eventID, session.WorkspaceID, linkID, defaultDomain.ID, "claim_"+linkID,
		now, make([]byte, 32),
	); err != nil {
		t.Fatalf("创建游客点击失败: %v", err)
	}

	result, err := repo.Claim(t.Context(), session.TokenHash, targetWorkspaceID, now)
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if result.Links != 1 || result.Clicks != 1 || len(result.CacheKeys) != 1 {
		t.Fatalf("Claim() result = %+v", result)
	}

	var movedWorkspaceID string
	var movedExpiresAt sql.NullTime
	if err := db.QueryRowContext(t.Context(), `
		SELECT workspace_id, expires_at FROM links WHERE id = ?`, linkID,
	).Scan(&movedWorkspaceID, &movedExpiresAt); err != nil {
		t.Fatalf("查询认领后链接失败: %v", err)
	}
	if movedWorkspaceID != targetWorkspaceID || movedExpiresAt.Valid {
		t.Fatalf("认领后链接 workspace/expires = %q/%v", movedWorkspaceID, movedExpiresAt)
	}

	var clickWorkspaceID string
	if err := db.QueryRowContext(t.Context(), `
		SELECT workspace_id FROM click_events WHERE event_id = ?`, eventID,
	).Scan(&clickWorkspaceID); err != nil {
		t.Fatalf("查询认领后点击失败: %v", err)
	}
	if clickWorkspaceID != targetWorkspaceID {
		t.Fatalf("认领后点击 workspace = %q, want %q", clickWorkspaceID, targetWorkspaceID)
	}

	lateEventID := ulid.Make().String()
	if _, err := analytics.NewMySQLRepository(db).InsertBatch(t.Context(), []analytics.Event{{
		EventID:     lateEventID,
		WorkspaceID: session.WorkspaceID,
		LinkID:      linkID,
		DomainID:    defaultDomain.ID,
		Slug:        "claim_" + linkID,
		ClickedAt:   now,
		VisitorHash: make([]byte, 32),
	}}); err != nil {
		t.Fatalf("写入认领前入队点击失败: %v", err)
	}
	var lateWorkspaceID string
	if err := db.GetContext(t.Context(), &lateWorkspaceID, `
		SELECT workspace_id FROM click_events WHERE event_id = ?`, lateEventID); err != nil {
		t.Fatalf("查询认领前入队点击失败: %v", err)
	}
	if lateWorkspaceID != targetWorkspaceID {
		t.Fatalf("迟到点击 workspace = %q, want %q", lateWorkspaceID, targetWorkspaceID)
	}

	var sessionCount int
	if err := db.GetContext(t.Context(), &sessionCount, `
		SELECT COUNT(*) FROM trial_sessions WHERE id = ?`, session.ID); err != nil {
		t.Fatalf("统计游客会话失败: %v", err)
	}
	var trialWorkspaceCount int
	if err := db.GetContext(t.Context(), &trialWorkspaceCount, `
		SELECT COUNT(*) FROM workspaces WHERE id = ?`, session.WorkspaceID); err != nil {
		t.Fatalf("统计游客工作空间失败: %v", err)
	}
	if sessionCount != 0 || trialWorkspaceCount != 0 {
		t.Fatalf("认领后 session/workspace = %d/%d", sessionCount, trialWorkspaceCount)
	}
}
