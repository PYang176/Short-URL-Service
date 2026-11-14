//go:build integration

package abuse

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

// TestMySQLRepositoryGovernanceLifecycle 验证黑名单、举报下架和审计事务
func TestMySQLRepositoryGovernanceLifecycle(t *testing.T) {
	db := openIntegrationDatabase(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	adminID, _, workspaceID := createIntegrationIdentity(t, db, now)
	domainID, linkID := createIntegrationLink(t, db, workspaceID, now)
	repo := NewMySQLRepository(db)

	hostname := "blocked-" + ulid.Make().String() + ".example"
	domain := &BlockedDomain{
		ID:              ulid.Make().String(),
		Hostname:        hostname,
		Reason:          "钓鱼域名",
		CreatedByUserID: adminID,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := repo.AddBlockedDomain(t.Context(), domain); err != nil {
		t.Fatalf("AddBlockedDomain() error = %v", err)
	}

	tests := []struct {
		name     string
		host     string
		expected bool
	}{
		{name: "精确匹配", host: hostname, expected: true},
		{name: "子域名匹配", host: "login." + hostname, expected: true},
		{name: "其他域名", host: "example.org", expected: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			blocked, err := repo.IsDestinationBlocked(t.Context(), test.host)
			if err != nil {
				t.Fatalf("IsDestinationBlocked() error = %v", err)
			}
			if blocked != test.expected {
				t.Fatalf("blocked = %v, want %v", blocked, test.expected)
			}
		})
	}
	if err := repo.RemoveBlockedDomain(t.Context(), domain.ID, adminID, now.Add(time.Second)); err != nil {
		t.Fatalf("RemoveBlockedDomain() error = %v", err)
	}
	if blocked, err := repo.IsDestinationBlocked(t.Context(), hostname); err != nil || blocked {
		t.Fatalf("移除后 blocked=%v error=%v", blocked, err)
	}

	report := &Report{
		ID:          ulid.Make().String(),
		LinkID:      linkID,
		WorkspaceID: workspaceID,
		Reason:      ReasonPhishing,
		Details:     "测试举报",
		Status:      ReportStatusPending,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := repo.CreateReport(t.Context(), report); err != nil {
		t.Fatalf("CreateReport() error = %v", err)
	}
	reports, err := repo.ListReports(t.Context(), ReportStatusPending, 1000)
	if err != nil {
		t.Fatalf("ListReports() error = %v", err)
	}
	found := false
	for _, item := range reports {
		if item.ID != report.ID {
			continue
		}
		found = true
		if item.LinkSlug != "abuse-link" {
			t.Fatalf("ListReports() 当前举报关联信息 = %+v", item)
		}
		break
	}
	if !found {
		t.Fatalf("ListReports() 未返回当前举报 %s", report.ID)
	}

	resolution, err := repo.ResolveReport(
		t.Context(),
		report.ID,
		adminID,
		DecisionTakedown,
		"确认违规",
		now.Add(2*time.Second),
	)
	if err != nil {
		t.Fatalf("ResolveReport() error = %v", err)
	}
	if resolution.DomainID != domainID || resolution.Slug != "abuse-link" {
		t.Fatalf("ResolveReport() = %+v", resolution)
	}

	var linkStatus uint8
	if err := db.GetContext(t.Context(), &linkStatus, `
		SELECT status FROM links WHERE id = ?`, linkID); err != nil {
		t.Fatalf("查询短链状态失败: %v", err)
	}
	if linkStatus != 2 {
		t.Fatalf("短链状态 = %d, want disabled", linkStatus)
	}
	var auditCount int
	if err := db.GetContext(t.Context(), &auditCount, `
		SELECT COUNT(*) FROM audit_logs
		WHERE actor_user_id = ? AND action = 'abuse.report.takedown'`,
		adminID,
	); err != nil {
		t.Fatalf("查询审计日志失败: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("下架审计数 = %d, want 1", auditCount)
	}

	if _, err := repo.ResolveReport(
		t.Context(),
		report.ID,
		adminID,
		DecisionReject,
		"重复处理",
		now.Add(3*time.Second),
	); !errors.Is(err, ErrAlreadyResolved) {
		t.Fatalf("重复 ResolveReport() error = %v, want ErrAlreadyResolved", err)
	}
}

// openIntegrationDatabase 打开真实 MySQL 测试连接
func openIntegrationDatabase(t *testing.T) *sqlx.DB {
	t.Helper()
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
	return db
}

// createIntegrationIdentity 创建管理员、普通用户和工作空间
func createIntegrationIdentity(
	t *testing.T,
	db *sqlx.DB,
	now time.Time,
) (string, string, string) {
	t.Helper()
	adminID := ulid.Make().String()
	ownerID := ulid.Make().String()
	workspaceID := ulid.Make().String()
	for index, userID := range []string{adminID, ownerID} {
		role := 1
		if index == 0 {
			role = 2
		}
		if _, err := db.ExecContext(t.Context(), `
			INSERT INTO users (
				id, email, password_hash, display_name, role, status, created_at, updated_at
			) VALUES (?, ?, 'test-hash', 'Abuse Integration', ?, 1, ?, ?)`,
			userID, "abuse-"+userID+"@example.com", role, now, now,
		); err != nil {
			t.Fatalf("创建测试用户失败: %v", err)
		}
	}
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO workspaces (id, name, slug, owner_user_id, created_at, updated_at)
		VALUES (?, 'Abuse Integration', ?, ?, ?, ?)`,
		workspaceID, "abuse-"+workspaceID, ownerID, now, now,
	); err != nil {
		t.Fatalf("创建测试工作空间失败: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM audit_logs WHERE actor_user_id IN (?, ?)`, adminID, ownerID)
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM abuse_reports WHERE workspace_id = ?`, workspaceID)
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM blocked_domains WHERE created_by_user_id IN (?, ?)`, adminID, ownerID)
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM links WHERE workspace_id = ?`, workspaceID)
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM domains WHERE workspace_id = ?`, workspaceID)
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM workspaces WHERE id = ?`, workspaceID)
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM users WHERE id IN (?, ?)`, adminID, ownerID)
	})
	return adminID, ownerID, workspaceID
}

// createIntegrationLink 创建举报测试使用的域名和短链
func createIntegrationLink(
	t *testing.T,
	db *sqlx.DB,
	workspaceID string,
	now time.Time,
) (string, string) {
	t.Helper()
	domainID := ulid.Make().String()
	linkID := ulid.Make().String()
	hostname := "links-" + workspaceID + ".test"
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO domains (
			id, workspace_id, hostname, status, created_at, updated_at
		) VALUES (?, ?, ?, 3, ?, ?)`,
		domainID, workspaceID, hostname, now, now,
	); err != nil {
		t.Fatalf("创建测试域名失败: %v", err)
	}
	destination := "https://example.com/abuse"
	hash := sha256.Sum256([]byte(destination))
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO links (
			id, workspace_id, domain_id, slug, destination_url, destination_hash,
			title, notes, status, redirect_code, human_clicks, version, created_at, updated_at
		) VALUES (?, ?, ?, 'abuse-link', ?, ?, '举报测试', '', 1, 302, 0, 1, ?, ?)`,
		linkID, workspaceID, domainID, destination, hash[:], now, now,
	); err != nil {
		t.Fatalf("创建测试短链失败: %v", err)
	}
	return domainID, linkID
}
