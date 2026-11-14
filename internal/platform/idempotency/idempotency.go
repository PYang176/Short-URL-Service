package idempotency

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

const (
	// StateProcessing 表示首次请求仍在处理中
	StateProcessing uint8 = 1
	// StateCompleted 表示首次请求已有可重放结果
	StateCompleted uint8 = 2
	// processingLease 是允许重新接管未完成幂等请求的等待时间
	processingLease = 30 * time.Second
)

var (
	// ErrInProgress 表示相同幂等键的首次请求仍在执行
	ErrInProgress = errors.New("幂等请求正在处理中")
	// ErrRequestMismatch 表示相同幂等键使用了不同请求内容
	ErrRequestMismatch = errors.New("幂等键已用于其他请求")
)

// Record 是可重放的幂等请求结果
type Record struct {
	// RequestHash 是原始请求摘要
	RequestHash []byte `db:"request_hash"`
	// State 是 processing 或 completed
	State uint8 `db:"state"`
	// StatusCode 是首次成功响应状态
	StatusCode sql.NullInt64 `db:"status_code"`
	// ResponseBody 是首次成功响应体
	ResponseBody sql.NullString `db:"response_body"`
	// ExpiresAt 是幂等结果失效时间
	ExpiresAt time.Time `db:"expires_at"`
	// UpdatedAt 是 processing 状态最近一次领取时间
	UpdatedAt time.Time `db:"updated_at"`
}

// Repository 定义幂等状态持久化
type Repository interface {
	// Begin 领取幂等键或返回已有状态
	Begin(ctx context.Context, workspaceID, key string, hash []byte, now time.Time, ttl time.Duration) (*Record, bool, error)
	// Complete 保存首次成功响应
	Complete(ctx context.Context, workspaceID, key string, hash []byte, status int, body string, now time.Time) error
	// Abort 删除未完成的领取状态
	Abort(ctx context.Context, workspaceID, key string, hash []byte) error
}

// MySQLRepository 使用 MySQL 保存幂等状态
type MySQLRepository struct {
	db *sqlx.DB
}

// NewMySQLRepository 创建幂等仓库
func NewMySQLRepository(db *sqlx.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

// Begin 领取幂等键或返回已有状态
func (r *MySQLRepository) Begin(
	ctx context.Context,
	workspaceID, key string,
	hash []byte,
	now time.Time,
	ttl time.Duration,
) (*Record, bool, error) {
	if _, err := r.db.ExecContext(ctx, `
		DELETE FROM idempotency_keys
		WHERE workspace_id = ? AND idempotency_key = ? AND expires_at <= ?`,
		workspaceID, key, now,
	); err != nil {
		return nil, false, fmt.Errorf("清理过期幂等键失败: %w", err)
	}

	result, err := r.db.ExecContext(ctx, `
		INSERT IGNORE INTO idempotency_keys (
			workspace_id, idempotency_key, request_hash, state,
			created_at, updated_at, expires_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		workspaceID, key, hash, StateProcessing, now, now, now.Add(ttl),
	)
	if err != nil {
		return nil, false, fmt.Errorf("领取幂等键失败: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, false, fmt.Errorf("读取幂等键领取行数失败: %w", err)
	}

	var record Record
	if err := r.db.GetContext(ctx, &record, `
		SELECT request_hash, state, status_code, response_body, expires_at, updated_at
		FROM idempotency_keys
		WHERE workspace_id = ? AND idempotency_key = ?`,
		workspaceID, key,
	); err != nil {
		return nil, false, fmt.Errorf("查询幂等键失败: %w", err)
	}
	if !bytes.Equal(record.RequestHash, hash) {
		return nil, false, ErrRequestMismatch
	}
	if affected == 1 {
		return &record, true, nil
	}
	if record.State != StateProcessing || record.UpdatedAt.After(now.Add(-processingLease)) {
		return &record, false, nil
	}

	// 过期处理中状态允许相同请求接管；业务表自身的同一幂等键约束保证不会重复创建资源。

	result, err = r.db.ExecContext(ctx, `
		UPDATE idempotency_keys
		SET request_hash = ?, updated_at = ?, expires_at = ?
		WHERE workspace_id = ? AND idempotency_key = ?
		  AND state = ? AND updated_at <= ?`,
		hash, now, now.Add(ttl),
		workspaceID, key, StateProcessing, now.Add(-processingLease),
	)
	if err != nil {
		return nil, false, fmt.Errorf("接管过期幂等请求失败: %w", err)
	}
	reclaimed, err := result.RowsAffected()
	if err != nil {
		return nil, false, fmt.Errorf("读取幂等接管行数失败: %w", err)
	}
	if reclaimed == 1 {
		record.UpdatedAt = now
		record.ExpiresAt = now.Add(ttl)
		return &record, true, nil
	}
	return &record, false, nil
}

// Complete 保存首次成功响应
func (r *MySQLRepository) Complete(
	ctx context.Context,
	workspaceID, key string,
	hash []byte,
	status int,
	body string,
	now time.Time,
) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE idempotency_keys
		SET state = ?, status_code = ?, response_body = ?, updated_at = ?
		WHERE workspace_id = ? AND idempotency_key = ?
		  AND request_hash = ? AND state = ?`,
		StateCompleted, status, body, now,
		workspaceID, key, hash, StateProcessing,
	)
	if err != nil {
		return fmt.Errorf("保存幂等响应失败: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取幂等响应更新行数失败: %w", err)
	}
	if affected == 0 {
		return ErrInProgress
	}
	return nil
}

// Abort 删除未完成的领取状态
func (r *MySQLRepository) Abort(ctx context.Context, workspaceID, key string, hash []byte) error {
	if _, err := r.db.ExecContext(ctx, `
		DELETE FROM idempotency_keys
		WHERE workspace_id = ? AND idempotency_key = ?
		  AND request_hash = ? AND state = ?`,
		workspaceID, key, hash, StateProcessing,
	); err != nil {
		return fmt.Errorf("释放幂等键失败: %w", err)
	}
	return nil
}

// Service 提供幂等请求流程
type Service struct {
	// repo 是幂等状态仓库
	repo Repository
	// ttl 是结果保存时间
	ttl time.Duration
	// now 提供可替换的 UTC 时间
	now func() time.Time
}

// NewService 创建幂等服务
func NewService(repo Repository, ttl time.Duration) *Service {
	return &Service{
		repo: repo,
		ttl:  ttl,
		now:  func() time.Time { return time.Now().UTC() },
	}
}

// Begin 领取幂等键并返回是否为新请求
func (s *Service) Begin(
	ctx context.Context,
	workspaceID, key string,
	hash []byte,
) (*Record, bool, error) {
	return s.repo.Begin(ctx, workspaceID, key, hash, s.now(), s.ttl)
}

// Complete 保存成功响应
func (s *Service) Complete(
	ctx context.Context,
	workspaceID, key string,
	hash []byte,
	status int,
	body string,
) error {
	return s.repo.Complete(ctx, workspaceID, key, hash, status, body, s.now())
}

// Abort 释放未完成的幂等键
func (s *Service) Abort(ctx context.Context, workspaceID, key string, hash []byte) error {
	return s.repo.Abort(ctx, workspaceID, key, hash)
}
