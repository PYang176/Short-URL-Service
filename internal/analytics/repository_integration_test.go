//go:build integration

package analytics

import (
	"context"
	"crypto/sha256"
	"os"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/oklog/ulid/v2"

	"github.com/PYang176/Short-URL-Service/internal/link"
)

// TestMySQLRepositoryAnalytics 在真实 MySQL 中验证幂等写入和聚合查询
func TestMySQLRepositoryAnalytics(t *testing.T) {
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

	workspaceID := ulid.Make().String()
	linkID := ulid.Make().String()
	baseTime := time.Date(2026, 11, 12, 10, 5, 0, 0, time.UTC)
	visitorOne := sha256.Sum256([]byte("visitor-one"))
	visitorTwo := sha256.Sum256([]byte("visitor-two"))
	events := []Event{
		{
			EventID: ulid.Make().String(), WorkspaceID: workspaceID, LinkID: linkID,
			DomainID: "domain-1", Slug: "analytics", ClickedAt: baseTime,
			VisitorHash: visitorOne[:], Country: "CN", Device: "desktop",
			OS: "Windows", Browser: "Chrome", IsBot: false,
		},
		{
			EventID: ulid.Make().String(), WorkspaceID: workspaceID, LinkID: linkID,
			DomainID: "domain-1", Slug: "analytics", ClickedAt: baseTime.Add(5 * time.Minute),
			VisitorHash: visitorOne[:], Country: "US", Device: "mobile",
			OS: "Android", Browser: "Chrome", IsBot: false,
		},
		{
			EventID: ulid.Make().String(), WorkspaceID: workspaceID, LinkID: linkID,
			DomainID: "domain-1", Slug: "analytics", ClickedAt: baseTime.Add(10 * time.Minute),
			VisitorHash: visitorTwo[:], Country: "CN", Device: "desktop",
			OS: "Linux", Browser: "curl", IsBot: true,
		},
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM click_events WHERE workspace_id = ?`, workspaceID)
	})

	repo := NewMySQLRepository(db)
	inserted, err := repo.InsertBatch(t.Context(), events)
	if err != nil {
		t.Fatalf("InsertBatch() error = %v", err)
	}
	if inserted != 3 {
		t.Fatalf("首次写入数量 = %d, want 3", inserted)
	}
	inserted, err = repo.InsertBatch(t.Context(), events)
	if err != nil {
		t.Fatalf("重复 InsertBatch() error = %v", err)
	}
	if inserted != 0 {
		t.Fatalf("重复写入数量 = %d, want 0", inserted)
	}

	from := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 1)
	summary, err := repo.Summary(t.Context(), workspaceID, linkID, from, to)
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	if summary.TotalClicks != 3 || summary.HumanClicks != 2 ||
		summary.BotClicks != 1 || summary.UniqueVisitors != 1 {
		t.Fatalf("Summary() = %+v", summary)
	}

	trend, err := repo.Trend(t.Context(), workspaceID, linkID, from, to, false)
	if err != nil {
		t.Fatalf("Trend() error = %v", err)
	}
	if len(trend) != 1 || trend[0].HumanClicks != 2 || trend[0].BotClicks != 1 {
		t.Fatalf("Trend() = %+v", trend)
	}

	devices, err := repo.Breakdown(t.Context(), workspaceID, linkID, from, to, "device")
	if err != nil {
		t.Fatalf("Breakdown() error = %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("设备维度数量 = %d, want 2; %+v", len(devices), devices)
	}
}

// TestMySQLRepositoryWorkspaceOverview 验证工作空间聚合、热门链接和隔离
func TestMySQLRepositoryWorkspaceOverview(t *testing.T) {
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

	now := time.Date(2026, 11, 13, 12, 30, 0, 0, time.UTC)
	userID := ulid.Make().String()
	targetWorkspaceID := ulid.Make().String()
	otherWorkspaceID := ulid.Make().String()
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO users (id, email, password_hash, display_name, role, status, created_at, updated_at)
		VALUES (?, ?, 'test-hash', 'Overview Test', 1, 1, ?, ?)`,
		userID, "overview-"+userID+"@example.com", now, now,
	); err != nil {
		t.Fatalf("创建测试用户失败: %v", err)
	}
	for index, workspaceID := range []string{targetWorkspaceID, otherWorkspaceID} {
		if _, err := db.ExecContext(t.Context(), `
			INSERT INTO workspaces (id, kind, name, slug, owner_user_id, created_at, updated_at)
			VALUES (?, 1, ?, ?, ?, ?, ?)`,
			workspaceID, "Overview "+workspaceID, "overview_"+workspaceID, userID, now, now,
		); err != nil {
			t.Fatalf("创建工作空间 %d 失败: %v", index+1, err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM click_events WHERE workspace_id IN (?, ?)`, targetWorkspaceID, otherWorkspaceID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM links WHERE workspace_id IN (?, ?)`, targetWorkspaceID, otherWorkspaceID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM workspaces WHERE id IN (?, ?)`, targetWorkspaceID, otherWorkspaceID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id = ?`, userID)
	})

	linkRepo := link.NewMySQLRepository(db)
	domain, err := linkRepo.EnsureDefaultDomain(t.Context(), "overview.test", now)
	if err != nil {
		t.Fatalf("创建默认域名失败: %v", err)
	}
	activeLinkID := ulid.Make().String()
	disabledLinkID := ulid.Make().String()
	otherLinkID := ulid.Make().String()
	for _, item := range []struct {
		id          string
		workspaceID string
		status      uint8
		title       string
	}{
		{activeLinkID, targetWorkspaceID, link.StatusActive, "Active"},
		{disabledLinkID, targetWorkspaceID, link.StatusDisabled, "Disabled"},
		{otherLinkID, otherWorkspaceID, link.StatusActive, "Other"},
	} {
		if err := linkRepo.Create(t.Context(), &link.Link{
			ID:              item.id,
			WorkspaceID:     item.workspaceID,
			DomainID:        domain.ID,
			DomainHostname:  domain.Hostname,
			Slug:            "overview_" + item.id,
			DestinationURL:  "https://example.com/" + item.id,
			DestinationHash: make([]byte, 32),
			Title:           item.title,
			Status:          item.status,
			RedirectCode:    302,
			Version:         1,
			CreatedAt:       now,
			UpdatedAt:       now,
		}, nil); err != nil {
			t.Fatalf("创建链接 %s 失败: %v", item.title, err)
		}
	}

	repo := NewMySQLRepository(db)
	events := []Event{
		{
			EventID: ulid.Make().String(), WorkspaceID: targetWorkspaceID,
			LinkID: activeLinkID, DomainID: domain.ID, Slug: "overview_" + activeLinkID,
			ClickedAt: now.Add(-10 * time.Minute), VisitorHash: sha256Sum("overview-one"),
		},
		{
			EventID: ulid.Make().String(), WorkspaceID: targetWorkspaceID,
			LinkID: activeLinkID, DomainID: domain.ID, Slug: "overview_" + activeLinkID,
			ClickedAt: now.Add(-5 * time.Minute), VisitorHash: sha256Sum("overview-two"),
		},
		{
			EventID: ulid.Make().String(), WorkspaceID: targetWorkspaceID,
			LinkID: disabledLinkID, DomainID: domain.ID, Slug: "overview_" + disabledLinkID,
			ClickedAt: now.Add(-5 * time.Minute), VisitorHash: sha256Sum("overview-bot"), IsBot: true,
		},
		{
			EventID: ulid.Make().String(), WorkspaceID: otherWorkspaceID,
			LinkID: otherLinkID, DomainID: domain.ID, Slug: "overview_" + otherLinkID,
			ClickedAt: now.Add(-3 * time.Minute), VisitorHash: sha256Sum("overview-other"),
		},
	}
	if _, err := repo.InsertBatch(t.Context(), events); err != nil {
		t.Fatalf("写入点击事件失败: %v", err)
	}

	from := now.AddDate(0, 0, -7)
	summary, err := repo.WorkspaceSummary(t.Context(), targetWorkspaceID, now, from, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("WorkspaceSummary() error = %v", err)
	}
	if summary.TotalLinks != 2 || summary.ActiveLinks != 1 ||
		summary.TotalClicks != 3 || summary.HumanClicks != 2 ||
		summary.BotClicks != 1 || summary.UniqueVisitors != 2 {
		t.Fatalf("WorkspaceSummary() = %+v", summary)
	}

	trend, err := repo.WorkspaceTrend(t.Context(), targetWorkspaceID, from, now.Add(time.Minute), false)
	if err != nil {
		t.Fatalf("WorkspaceTrend() error = %v", err)
	}
	if len(trend) != 1 || trend[0].HumanClicks != 2 || trend[0].BotClicks != 1 {
		t.Fatalf("WorkspaceTrend() = %+v", trend)
	}

	topLinks, err := repo.TopLinks(t.Context(), targetWorkspaceID, from, now.Add(time.Minute), 5)
	if err != nil {
		t.Fatalf("TopLinks() error = %v", err)
	}
	if len(topLinks) != 2 || topLinks[0].ID != activeLinkID || topLinks[0].Clicks != 2 {
		t.Fatalf("TopLinks() = %+v", topLinks)
	}
	otherSummary, err := repo.WorkspaceSummary(t.Context(), otherWorkspaceID, now, from, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("other WorkspaceSummary() error = %v", err)
	}
	if otherSummary.TotalLinks != 1 || otherSummary.TotalClicks != 1 {
		t.Fatalf("other WorkspaceSummary() = %+v", otherSummary)
	}
}

// sha256Sum 生成固定测试访客摘要
func sha256Sum(value string) []byte {
	hash := sha256.Sum256([]byte(value))
	return hash[:]
}
