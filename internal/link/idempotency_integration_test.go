//go:build integration

package link

import (
	"context"
	"crypto/sha256"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/oklog/ulid/v2"

	"github.com/PYang176/Short-URL-Service/internal/platform/idempotency"
)

// TestCreateRecoversAfterIdempotencyRecordStall 验证业务已提交但幂等状态卡住时可以返回原链接。
func TestCreateRecoversAfterIdempotencyRecordStall(t *testing.T) {
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
		VALUES (?, ?, 'test-hash', 'Idempotency Recovery', 1, 1, ?, ?)`,
		userID, "idem-link-"+userID+"@example.com", now, now,
	); err != nil {
		t.Fatalf("创建测试用户失败: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO workspaces (id, kind, name, slug, owner_user_id, created_at, updated_at)
		VALUES (?, 1, 'Idempotency Recovery', ?, ?, ?, ?)`,
		workspaceID, "idem-link-"+workspaceID, userID, now, now,
	); err != nil {
		t.Fatalf("创建测试工作空间失败: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM idempotency_keys WHERE workspace_id = ?`, workspaceID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM links WHERE workspace_id = ?`, workspaceID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM domains WHERE workspace_id = ?`, workspaceID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM workspaces WHERE id = ?`, workspaceID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id = ?`, userID)
	})

	repo := NewMySQLRepository(db)
	service, err := NewService(
		repo,
		nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		"http://idempotency-recovery.test",
		nil,
	)
	if err != nil {
		t.Fatalf("创建链接服务失败: %v", err)
	}
	if err := service.Initialize(t.Context()); err != nil {
		t.Fatalf("初始化默认域名失败: %v", err)
	}
	idempotencyService := idempotency.NewService(
		idempotency.NewMySQLRepository(db),
		time.Hour,
	)
	key := "crash-recovery-key"
	input := CreateInput{
		DestinationURL: "https://example.com/idempotency-recovery",
		IdempotencyKey: key,
	}
	requestHash := sha256.Sum256([]byte(input.DestinationURL))
	if _, claimed, err := idempotencyService.Begin(
		t.Context(),
		workspaceID,
		key,
		requestHash[:],
	); err != nil || !claimed {
		t.Fatalf("首次 Begin() claimed=%v error=%v", claimed, err)
	}
	created, err := service.Create(t.Context(), workspaceID, input)
	if err != nil {
		t.Fatalf("首次 Create() error = %v", err)
	}

	if _, err := db.ExecContext(t.Context(), `
		UPDATE idempotency_keys
		SET updated_at = ?
		WHERE workspace_id = ? AND idempotency_key = ?`,
		time.Now().UTC().Add(-31*time.Second),
		workspaceID,
		key,
	); err != nil {
		t.Fatalf("模拟幂等状态停滞失败: %v", err)
	}
	if _, claimed, err := idempotencyService.Begin(
		t.Context(),
		workspaceID,
		key,
		requestHash[:],
	); err != nil || !claimed {
		t.Fatalf("接管 Begin() claimed=%v error=%v", claimed, err)
	}
	recovered, err := service.Create(t.Context(), workspaceID, input)
	if err != nil {
		t.Fatalf("恢复 Create() error = %v", err)
	}
	if recovered.ID != created.ID || recovered.Slug != created.Slug {
		t.Fatalf("恢复结果 = %+v, want id=%s slug=%s", recovered, created.ID, created.Slug)
	}
	if err := idempotencyService.Complete(
		t.Context(),
		workspaceID,
		key,
		requestHash[:],
		201,
		`{"id":"recovered"}`,
	); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
}
