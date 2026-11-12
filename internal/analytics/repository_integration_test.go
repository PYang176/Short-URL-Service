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
