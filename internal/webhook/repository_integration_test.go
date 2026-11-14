//go:build integration

package webhook

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/oklog/ulid/v2"

	"github.com/PYang176/Short-URL-Service/internal/link"
)

// TestMySQLRepositoryOutboxLifecycle 验证链接事务写入、任务领取和成功状态
func TestMySQLRepositoryOutboxLifecycle(t *testing.T) {
	db, workspaceID := integrationDatabase(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	cipher, err := NewSecretCipher(strings.Repeat("i", 32))
	if err != nil {
		t.Fatalf("NewSecretCipher() error = %v", err)
	}
	secret, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret() error = %v", err)
	}
	ciphertext, err := cipher.Encrypt(secret)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	repo := NewMySQLRepository(db)
	endpoint := &Endpoint{
		ID:               ulid.Make().String(),
		WorkspaceID:      workspaceID,
		Name:             "Integration Webhook",
		URL:              "http://127.0.0.1:9010/hook",
		SecretCiphertext: ciphertext,
		Status:           EndpointStatusActive,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := repo.CreateEndpoint(t.Context(), endpoint); err != nil {
		t.Fatalf("CreateEndpoint() error = %v", err)
	}

	linkRepo, domain := integrationLinkRepository(t, db, workspaceID, now)
	event := &link.OutboxEvent{
		ID:      ulid.Make().String(),
		Type:    "link.created",
		Payload: []byte(`{"id":"event","type":"link.created","data":{"slug":"outbox"}}`),
	}
	destination := "https://example.com/outbox"
	destinationHash := sha256.Sum256([]byte(destination))
	createdLink := &link.Link{
		ID:              ulid.Make().String(),
		WorkspaceID:     workspaceID,
		DomainID:        domain.ID,
		DomainHostname:  domain.Hostname,
		Slug:            "outbox",
		DestinationURL:  destination,
		DestinationHash: destinationHash[:],
		Status:          link.StatusActive,
		RedirectCode:    302,
		Version:         1,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := linkRepo.Create(t.Context(), createdLink, event); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	deliveries, err := repo.ListDeliveries(
		t.Context(),
		workspaceID,
		endpoint.ID,
		10,
	)
	if err != nil {
		t.Fatalf("ListDeliveries() error = %v", err)
	}
	if len(deliveries) != 1 || deliveries[0].EventID != event.ID {
		t.Fatalf("ListDeliveries() = %+v", deliveries)
	}

	jobs, err := repo.ClaimDeliveries(
		t.Context(),
		10,
		now.Add(time.Second),
		now.Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("ClaimDeliveries() error = %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("ClaimDeliveries() length = %d, want 1", len(jobs))
	}
	if jobs[0].AttemptCount != 1 || jobs[0].EndpointURL != endpoint.URL {
		t.Fatalf("ClaimDeliveries() job = %+v", jobs[0])
	}
	if err := repo.MarkSuccess(
		t.Context(),
		&jobs[0],
		204,
		now.Add(2*time.Second),
	); err != nil {
		t.Fatalf("MarkSuccess() error = %v", err)
	}

	deliveries, err = repo.ListDeliveries(
		t.Context(),
		workspaceID,
		endpoint.ID,
		10,
	)
	if err != nil {
		t.Fatalf("ListDeliveries() after success error = %v", err)
	}
	if deliveries[0].Status != DeliveryStatusSuccess ||
		!deliveries[0].ResponseStatus.Valid ||
		deliveries[0].ResponseStatus.Int64 != 204 {
		t.Fatalf("成功投递记录 = %+v", deliveries[0])
	}
}

// TestMySQLRepositoryFailureDisablesEndpoint 验证达到最大尝试次数后终止投递和停用端点
func TestMySQLRepositoryFailureDisablesEndpoint(t *testing.T) {
	db, workspaceID := integrationDatabase(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	repo := NewMySQLRepository(db)
	endpoint := &Endpoint{
		ID:               ulid.Make().String(),
		WorkspaceID:      workspaceID,
		Name:             "Failing Webhook",
		URL:              "http://127.0.0.1:9011/hook",
		SecretCiphertext: []byte("ciphertext"),
		Status:           EndpointStatusActive,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := repo.CreateEndpoint(t.Context(), endpoint); err != nil {
		t.Fatalf("CreateEndpoint() error = %v", err)
	}
	linkRepo, domain := integrationLinkRepository(t, db, workspaceID, now)
	event := &link.OutboxEvent{
		ID:      ulid.Make().String(),
		Type:    "link.created",
		Payload: []byte(`{"id":"event","type":"link.created"}`),
	}
	destination := "https://example.com/failure"
	destinationHash := sha256.Sum256([]byte(destination))
	if err := linkRepo.Create(t.Context(), &link.Link{
		ID:              ulid.Make().String(),
		WorkspaceID:     workspaceID,
		DomainID:        domain.ID,
		DomainHostname:  domain.Hostname,
		Slug:            "failure",
		DestinationURL:  destination,
		DestinationHash: destinationHash[:],
		Status:          link.StatusActive,
		RedirectCode:    302,
		Version:         1,
		CreatedAt:       now,
		UpdatedAt:       now,
	}, event); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	jobs, err := repo.ClaimDeliveries(
		t.Context(),
		1,
		now.Add(time.Second),
		now.Add(time.Minute),
	)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("ClaimDeliveries() jobs=%+v error=%v", jobs, err)
	}
	status := 503
	if err := repo.MarkFailure(
		t.Context(),
		&jobs[0],
		&status,
		"目标服务不可用",
		now.Add(2*time.Second),
		2,
	); err != nil {
		t.Fatalf("MarkFailure() error = %v", err)
	}
	retryJobs, err := repo.ClaimDeliveries(
		t.Context(),
		1,
		now.Add(10*time.Second),
		now.Add(time.Minute),
	)
	if err != nil || len(retryJobs) != 1 {
		t.Fatalf("重试 ClaimDeliveries() jobs=%+v error=%v", retryJobs, err)
	}
	if err := repo.MarkFailure(
		t.Context(),
		&retryJobs[0],
		nil,
		"连接超时",
		now.Add(11*time.Second),
		2,
	); err != nil {
		t.Fatalf("第二次 MarkFailure() error = %v", err)
	}

	deliveries, err := repo.ListDeliveries(
		t.Context(),
		workspaceID,
		endpoint.ID,
		10,
	)
	if err != nil {
		t.Fatalf("ListDeliveries() error = %v", err)
	}
	if deliveries[0].Status != DeliveryStatusDead {
		t.Fatalf("失败投递状态 = %d, want dead", deliveries[0].Status)
	}
	if deliveries[0].ResponseStatus.Valid {
		t.Fatalf("网络失败不应保存 HTTP 状态: %+v", deliveries[0].ResponseStatus)
	}
	endpoints, err := repo.ListEndpoints(t.Context(), workspaceID)
	if err != nil {
		t.Fatalf("ListEndpoints() error = %v", err)
	}
	if len(endpoints) != 1 ||
		endpoints[0].Status != EndpointStatusDisabled ||
		endpoints[0].DisabledAt == nil {
		t.Fatalf("自动停用端点 = %+v", endpoints)
	}
}

// TestMySQLRepositoryConcurrentClaims 验证并发 Worker 不会领取同一条任务
func TestMySQLRepositoryConcurrentClaims(t *testing.T) {
	db, workspaceID := integrationDatabase(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	repo := NewMySQLRepository(db)
	endpoint := &Endpoint{
		ID:               ulid.Make().String(),
		WorkspaceID:      workspaceID,
		Name:             "Concurrent Webhook",
		URL:              "http://127.0.0.1:9012/hook",
		SecretCiphertext: []byte("ciphertext"),
		Status:           EndpointStatusActive,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := repo.CreateEndpoint(t.Context(), endpoint); err != nil {
		t.Fatalf("CreateEndpoint() error = %v", err)
	}
	linkRepo, domain := integrationLinkRepository(t, db, workspaceID, now)
	for index := 0; index < 2; index++ {
		destination := "https://example.com/concurrent-" + string(rune('a'+index))
		destinationHash := sha256.Sum256([]byte(destination))
		slug := "concurrent-" + string(rune('a'+index))
		event := &link.OutboxEvent{
			ID:      ulid.Make().String(),
			Type:    "link.created",
			Payload: []byte(`{"type":"link.created"}`),
		}
		if err := linkRepo.Create(t.Context(), &link.Link{
			ID:              ulid.Make().String(),
			WorkspaceID:     workspaceID,
			DomainID:        domain.ID,
			DomainHostname:  domain.Hostname,
			Slug:            slug,
			DestinationURL:  destination,
			DestinationHash: destinationHash[:],
			Status:          link.StatusActive,
			RedirectCode:    302,
			Version:         1,
			CreatedAt:       now,
			UpdatedAt:       now,
		}, event); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	type claimResult struct {
		jobs []DeliveryJob
		err  error
	}
	results := make(chan claimResult, 2)
	var wait sync.WaitGroup
	start := make(chan struct{})
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			repo := NewMySQLRepository(db)
			claimed := make([]DeliveryJob, 0, 2)
			for attempt := 0; attempt < 2; attempt++ {
				jobs, err := repo.ClaimDeliveries(
					context.Background(),
					1,
					now.Add(time.Second),
					now.Add(time.Minute),
				)
				if err != nil {
					results <- claimResult{err: err}
					return
				}
				claimed = append(claimed, jobs...)
			}
			results <- claimResult{jobs: claimed}
		}()
	}
	close(start)
	wait.Wait()
	close(results)

	seen := make(map[uint64]struct{}, 2)
	for result := range results {
		if result.err != nil {
			t.Fatalf("并发 ClaimDeliveries() error = %v", result.err)
		}
		for _, job := range result.jobs {
			if _, exists := seen[job.ID]; exists {
				t.Fatalf("同一投递任务被并发领取: %d", job.ID)
			}
			seen[job.ID] = struct{}{}
		}
	}
	if len(seen) != 2 {
		t.Fatalf("并发领取任务数 = %d, want 2", len(seen))
	}
}

// TestMySQLRepositoryConcurrentDisableAndMarkSuccess 验证停用端点与成功回写不会发生锁顺序死锁
func TestMySQLRepositoryConcurrentDisableAndMarkSuccess(t *testing.T) {
	db, workspaceID := integrationDatabase(t)
	repo := NewMySQLRepository(db)
	linkRepo, domain := integrationLinkRepository(
		t,
		db,
		workspaceID,
		time.Now().UTC().Truncate(time.Millisecond),
	)

	for index := 0; index < 80; index++ {
		now := time.Now().UTC().Truncate(time.Millisecond)
		endpoint := &Endpoint{
			ID:               ulid.Make().String(),
			WorkspaceID:      workspaceID,
			Name:             fmt.Sprintf("Lock Order %d", index),
			URL:              "http://127.0.0.1:9014/hook",
			SecretCiphertext: []byte("ciphertext"),
			Status:           EndpointStatusActive,
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		if err := repo.CreateEndpoint(t.Context(), endpoint); err != nil {
			t.Fatalf("CreateEndpoint() error = %v", err)
		}

		destination := fmt.Sprintf("https://example.com/lock-order-%d", index)
		destinationHash := sha256.Sum256([]byte(destination))
		event := &link.OutboxEvent{
			ID:      ulid.Make().String(),
			Type:    "link.created",
			Payload: []byte(`{"type":"link.created"}`),
		}
		if err := linkRepo.Create(t.Context(), &link.Link{
			ID:              ulid.Make().String(),
			WorkspaceID:     workspaceID,
			DomainID:        domain.ID,
			DomainHostname:  domain.Hostname,
			Slug:            fmt.Sprintf("lock-%d", index),
			DestinationURL:  destination,
			DestinationHash: destinationHash[:],
			Status:          link.StatusActive,
			RedirectCode:    302,
			Version:         1,
			CreatedAt:       now,
			UpdatedAt:       now,
		}, event); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		jobs, err := repo.ClaimDeliveries(
			t.Context(),
			1,
			now.Add(time.Second),
			now.Add(time.Minute),
		)
		if err != nil || len(jobs) != 1 {
			t.Fatalf("ClaimDeliveries() jobs=%d error=%v", len(jobs), err)
		}

		start := make(chan struct{})
		results := make(chan error, 2)
		var wait sync.WaitGroup
		wait.Add(2)
		go func(job *DeliveryJob) {
			defer wait.Done()
			<-start
			results <- repo.MarkSuccess(
				t.Context(),
				job,
				http.StatusNoContent,
				now.Add(2*time.Second),
			)
		}(&jobs[0])
		go func() {
			defer wait.Done()
			<-start
			results <- repo.DisableEndpoint(
				t.Context(),
				workspaceID,
				endpoint.ID,
				now.Add(2*time.Second),
			)
		}()
		close(start)
		wait.Wait()
		close(results)

		for resultErr := range results {
			if resultErr == nil || errors.Is(resultErr, ErrLeaseLost) {
				continue
			}
			var mysqlErr *driver.MySQLError
			if errors.As(resultErr, &mysqlErr) && mysqlErr.Number == 1213 {
				t.Fatalf("检测到事务死锁: %v", resultErr)
			}
			t.Fatalf("并发事务失败: %v", resultErr)
		}
	}
}

// TestMySQLRepositoryWorkspaceIsolation 验证投递记录不能跨工作空间读取
func TestMySQLRepositoryWorkspaceIsolation(t *testing.T) {
	db, workspaceID := integrationDatabase(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	repo := NewMySQLRepository(db)
	endpoint := &Endpoint{
		ID:               ulid.Make().String(),
		WorkspaceID:      workspaceID,
		Name:             "Private Webhook",
		URL:              "http://127.0.0.1:9013/hook",
		SecretCiphertext: []byte("ciphertext"),
		Status:           EndpointStatusActive,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := repo.CreateEndpoint(t.Context(), endpoint); err != nil {
		t.Fatalf("CreateEndpoint() error = %v", err)
	}
	if _, err := repo.ListDeliveries(
		t.Context(),
		ulid.Make().String(),
		endpoint.ID,
		10,
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("跨工作空间 ListDeliveries() error = %v, want ErrNotFound", err)
	}
	if err := repo.DisableEndpoint(
		t.Context(),
		ulid.Make().String(),
		endpoint.ID,
		now,
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("跨工作空间 DisableEndpoint() error = %v, want ErrNotFound", err)
	}
}

// integrationDatabase 创建隔离用户和工作空间，并注册资源清理
func integrationDatabase(t *testing.T) (*sqlx.DB, string) {
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

	now := time.Now().UTC().Truncate(time.Millisecond)
	userID := ulid.Make().String()
	workspaceID := ulid.Make().String()
	email := "webhook-" + userID + "@example.com"
	slug := "webhook-" + workspaceID
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO users (id, email, password_hash, display_name, role, status, created_at, updated_at)
		VALUES (?, ?, 'test-hash', 'Webhook Integration', 1, 1, ?, ?)`,
		userID, email, now, now,
	); err != nil {
		t.Fatalf("创建测试用户失败: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO workspaces (id, name, slug, owner_user_id, created_at, updated_at)
		VALUES (?, 'Webhook Integration', ?, ?, ?, ?)`,
		workspaceID, slug, userID, now, now,
	); err != nil {
		t.Fatalf("创建测试工作空间失败: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM webhook_deliveries WHERE workspace_id = ?`, workspaceID)
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM webhook_endpoints WHERE workspace_id = ?`, workspaceID)
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM links WHERE workspace_id = ?`, workspaceID)
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM domains WHERE workspace_id = ?`, workspaceID)
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM workspaces WHERE id = ?`, workspaceID)
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM users WHERE id = ?`, userID)
	})
	return db, workspaceID
}

// integrationLinkRepository 创建测试专用默认域名和链接仓库
func integrationLinkRepository(
	t *testing.T,
	db *sqlx.DB,
	workspaceID string,
	now time.Time,
) (*link.MySQLRepository, *link.Domain) {
	t.Helper()
	repo := link.NewMySQLRepository(db)
	domain, err := repo.EnsureDefaultDomain(
		t.Context(),
		"webhook-"+workspaceID+".test",
		now,
	)
	if err != nil {
		t.Fatalf("EnsureDefaultDomain() error = %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM links WHERE workspace_id = ?`, workspaceID)
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM domains WHERE id = ?`, domain.ID)
	})
	return repo, domain
}
