package domain

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

// Repository 定义自定义域名持久化能力
type Repository interface {
	// Create 保存待验证域名
	Create(ctx context.Context, domain *Domain) error
	// ListByWorkspace 返回工作空间未删除域名
	ListByWorkspace(ctx context.Context, workspaceID string) ([]Domain, error)
	// FindByID 返回工作空间内未删除域名
	FindByID(ctx context.Context, workspaceID, id string) (*Domain, error)
	// FindVerifiedByHostname 返回已验证的 Host
	FindVerifiedByHostname(ctx context.Context, hostname string) (*Domain, error)
	// UpdateVerification 更新验证状态
	UpdateVerification(ctx context.Context, domain *Domain) error
	// SoftDelete 软删除域名
	SoftDelete(ctx context.Context, workspaceID, id string, now time.Time) error
	// CountLinks 统计使用域名的未删除链接
	CountLinks(ctx context.Context, domainID string) (int, error)
}

// MySQLRepository 使用 MySQL 持久化域名
type MySQLRepository struct {
	db *sqlx.DB
}

// NewMySQLRepository 创建域名仓库
func NewMySQLRepository(db *sqlx.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

// Create 保存待验证域名
func (r *MySQLRepository) Create(ctx context.Context, domain *Domain) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO domains (
			id, workspace_id, hostname, status, verification_token,
			expected_cname, verification_attempts, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		domain.ID, domain.WorkspaceID, domain.Hostname, domain.Status,
		domain.VerificationToken, domain.ExpectedCNAME, domain.VerificationAttempts,
		domain.CreatedAt, domain.UpdatedAt,
	)
	if err != nil {
		var mysqlErr *driver.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return ErrDomainExists
		}
		return fmt.Errorf("创建域名失败: %w", err)
	}
	return nil
}

// ListByWorkspace 返回工作空间未删除域名
func (r *MySQLRepository) ListByWorkspace(ctx context.Context, workspaceID string) ([]Domain, error) {
	domains := make([]Domain, 0)
	err := r.db.SelectContext(ctx, &domains, `
		SELECT id, workspace_id, hostname, status, verification_token,
			expected_cname, verification_attempts, last_verified_at,
			next_verify_at, created_at, updated_at, deleted_at
		FROM domains
		WHERE workspace_id = ? AND deleted_at IS NULL
		ORDER BY created_at DESC`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("查询域名列表失败: %w", err)
	}
	return domains, nil
}

// FindByID 返回工作空间内未删除域名
func (r *MySQLRepository) FindByID(ctx context.Context, workspaceID, id string) (*Domain, error) {
	var result Domain
	err := r.db.GetContext(ctx, &result, `
		SELECT id, workspace_id, hostname, status, verification_token,
			expected_cname, verification_attempts, last_verified_at,
			next_verify_at, created_at, updated_at, deleted_at
		FROM domains
		WHERE workspace_id = ? AND id = ? AND deleted_at IS NULL
		LIMIT 1`, workspaceID, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询域名失败: %w", err)
	}
	return &result, nil
}

// FindVerifiedByHostname 返回已验证的 Host
func (r *MySQLRepository) FindVerifiedByHostname(ctx context.Context, hostname string) (*Domain, error) {
	var result Domain
	err := r.db.GetContext(ctx, &result, `
		SELECT id, workspace_id, hostname, status, verification_token,
			expected_cname, verification_attempts, last_verified_at,
			next_verify_at, created_at, updated_at, deleted_at
		FROM domains
		WHERE hostname = ? AND status = ? AND deleted_at IS NULL
		LIMIT 1`, hostname, StatusVerified)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("按 Host 查询域名失败: %w", err)
	}
	return &result, nil
}

// UpdateVerification 更新验证状态
func (r *MySQLRepository) UpdateVerification(ctx context.Context, domain *Domain) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE domains
		SET status = ?, verification_attempts = ?, last_verified_at = ?,
			next_verify_at = ?, updated_at = ?
		WHERE workspace_id = ? AND id = ? AND deleted_at IS NULL`,
		domain.Status, domain.VerificationAttempts, domain.LastVerifiedAt,
		domain.NextVerifyAt, domain.UpdatedAt, domain.WorkspaceID, domain.ID,
	)
	if err != nil {
		return fmt.Errorf("更新域名验证状态失败: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取域名更新行数失败: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// SoftDelete 软删除域名
func (r *MySQLRepository) SoftDelete(ctx context.Context, workspaceID, id string, now time.Time) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE domains SET deleted_at = ?, updated_at = ?, status = ?
		WHERE workspace_id = ? AND id = ? AND deleted_at IS NULL`,
		now, now, StatusDisabled, workspaceID, id,
	)
	if err != nil {
		return fmt.Errorf("删除域名失败: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取域名删除行数失败: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// CountLinks 统计使用域名的未删除链接
func (r *MySQLRepository) CountLinks(ctx context.Context, domainID string) (int, error) {
	var count int
	if err := r.db.GetContext(ctx, &count, `
		SELECT COUNT(*) FROM links WHERE domain_id = ? AND deleted_at IS NULL`,
		domainID,
	); err != nil {
		return 0, fmt.Errorf("统计域名链接失败: %w", err)
	}
	return count, nil
}
