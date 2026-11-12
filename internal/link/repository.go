package link

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// Repository 定义短链接持久化所需的最小能力
type Repository interface {
	// EnsureDefaultDomain 查询或创建平台默认域名
	EnsureDefaultDomain(ctx context.Context, hostname string, now time.Time) (*Domain, error)
	// Create 保存新短链接
	Create(ctx context.Context, link *Link) error
	// FindByID 按工作空间和链接 ID 查询未删除链接
	FindByID(ctx context.Context, workspaceID, id string) (*Link, error)
	// FindByDomainSlug 按域名和短码查询未删除链接
	FindByDomainSlug(ctx context.Context, domainID, slug string) (*Link, error)
	// List 按工作空间和游标倒序查询链接
	List(ctx context.Context, workspaceID string, limit int, beforeCreatedAt *time.Time, beforeID string) ([]Link, error)
	// Update 保存链接的当前可编辑字段并递增版本
	Update(ctx context.Context, link *Link) error
	// SoftDelete 软删除链接
	SoftDelete(ctx context.Context, workspaceID, id string, now time.Time) error
}

// MySQLRepository 使用 MySQL 持久化域名和链接
type MySQLRepository struct {
	db *sqlx.DB
}

type domainRow struct {
	ID          string         `db:"id"`
	WorkspaceID sql.NullString `db:"workspace_id"`
	Hostname    string         `db:"hostname"`
	Status      uint8          `db:"status"`
	CreatedAt   time.Time      `db:"created_at"`
	UpdatedAt   time.Time      `db:"updated_at"`
}

// NewMySQLRepository 创建短链接 MySQL 仓库
func NewMySQLRepository(db *sqlx.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

// EnsureDefaultDomain 查询或创建平台默认域名
func (r *MySQLRepository) EnsureDefaultDomain(ctx context.Context, hostname string, now time.Time) (*Domain, error) {
	const selectDomain = `
		SELECT id, workspace_id, hostname, status, created_at, updated_at
		FROM domains
		WHERE hostname = ? AND workspace_id IS NULL
		LIMIT 1`

	var row domainRow
	err := r.db.GetContext(ctx, &row, selectDomain, hostname)
	if err == nil {
		return toDomain(row), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("查询默认域名失败: %w", err)
	}

	domain := &Domain{
		ID:        newID(),
		Hostname:  hostname,
		Status:    DomainStatusVerified,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO domains (id, workspace_id, hostname, status, created_at, updated_at)
		VALUES (?, NULL, ?, ?, ?, ?)`,
		domain.ID, domain.Hostname, domain.Status, domain.CreatedAt, domain.UpdatedAt,
	)
	if err == nil {
		return domain, nil
	}
	if !isDuplicateKey(err) {
		return nil, fmt.Errorf("创建默认域名失败: %w", err)
	}

	// 并发启动时另一个实例可能已创建同一 Host，重新查询并复用。

	if err := r.db.GetContext(ctx, &row, selectDomain, hostname); err != nil {
		return nil, fmt.Errorf("重新查询默认域名失败: %w", err)
	}
	return toDomain(row), nil
}

// Create 保存新短链接
func (r *MySQLRepository) Create(ctx context.Context, link *Link) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO links (
			id, workspace_id, domain_id, slug, destination_url, destination_hash,
			title, notes, status, redirect_code, starts_at, expires_at,
			human_clicks, version, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		link.ID, link.WorkspaceID, link.DomainID, link.Slug, link.DestinationURL,
		link.DestinationHash, link.Title, link.Notes, link.Status, link.RedirectCode,
		link.StartsAt, link.ExpiresAt, link.HumanClicks, link.Version,
		link.CreatedAt, link.UpdatedAt,
	)
	if err != nil {
		if isDuplicateKey(err) {
			return ErrSlugTaken
		}
		return fmt.Errorf("创建短链接失败: %w", err)
	}
	return nil
}

// FindByID 按工作空间和链接 ID 查询未删除链接
func (r *MySQLRepository) FindByID(ctx context.Context, workspaceID, id string) (*Link, error) {
	var result Link
	err := r.db.GetContext(ctx, &result, `
		SELECT id, workspace_id, domain_id, slug, destination_url, destination_hash,
			title, notes, status, redirect_code, starts_at, expires_at,
			human_clicks, version, created_at, updated_at, deleted_at
		FROM links
		WHERE workspace_id = ? AND id = ? AND deleted_at IS NULL
		LIMIT 1`, workspaceID, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询短链接失败: %w", err)
	}
	return &result, nil
}

// FindByDomainSlug 按域名和短码查询未删除链接
func (r *MySQLRepository) FindByDomainSlug(ctx context.Context, domainID, slug string) (*Link, error) {
	var result Link
	err := r.db.GetContext(ctx, &result, `
		SELECT id, workspace_id, domain_id, slug, destination_url, destination_hash,
			title, notes, status, redirect_code, starts_at, expires_at,
			human_clicks, version, created_at, updated_at, deleted_at
		FROM links
		WHERE domain_id = ? AND slug = ? AND deleted_at IS NULL
		LIMIT 1`, domainID, slug)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("按短码查询链接失败: %w", err)
	}
	return &result, nil
}

// List 按工作空间和游标倒序查询链接
func (r *MySQLRepository) List(
	ctx context.Context,
	workspaceID string,
	limit int,
	beforeCreatedAt *time.Time,
	beforeID string,
) ([]Link, error) {
	query := `
		SELECT id, workspace_id, domain_id, slug, destination_url, destination_hash,
			title, notes, status, redirect_code, starts_at, expires_at,
			human_clicks, version, created_at, updated_at, deleted_at
		FROM links
		WHERE workspace_id = ? AND deleted_at IS NULL
		ORDER BY created_at DESC, id DESC
		LIMIT ?`
	args := []any{workspaceID, limit}
	if beforeCreatedAt != nil && beforeID != "" {
		query = `
		SELECT id, workspace_id, domain_id, slug, destination_url, destination_hash,
			title, notes, status, redirect_code, starts_at, expires_at,
			human_clicks, version, created_at, updated_at, deleted_at
		FROM links
		WHERE workspace_id = ?
		  AND deleted_at IS NULL
		  AND (created_at, id) < (?, ?)
		ORDER BY created_at DESC, id DESC
		LIMIT ?`
		args = []any{workspaceID, *beforeCreatedAt, beforeID, limit}
	}

	links := make([]Link, 0, limit)
	if err := r.db.SelectContext(ctx, &links, query, args...); err != nil {
		return nil, fmt.Errorf("查询短链接列表失败: %w", err)
	}
	return links, nil
}

// Update 保存链接的当前可编辑字段并递增版本
func (r *MySQLRepository) Update(ctx context.Context, link *Link) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE links
		SET destination_url = ?,
			destination_hash = ?,
			title = ?,
			notes = ?,
			status = ?,
			redirect_code = ?,
			starts_at = ?,
			expires_at = ?,
			version = ?,
			updated_at = ?
		WHERE workspace_id = ? AND id = ? AND deleted_at IS NULL`,
		link.DestinationURL, link.DestinationHash, link.Title, link.Notes,
		link.Status, link.RedirectCode, link.StartsAt, link.ExpiresAt,
		link.Version, link.UpdatedAt, link.WorkspaceID, link.ID,
	)
	if err != nil {
		return fmt.Errorf("更新短链接失败: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取短链接更新行数失败: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// SoftDelete 软删除链接
func (r *MySQLRepository) SoftDelete(ctx context.Context, workspaceID, id string, now time.Time) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE links
		SET deleted_at = ?, updated_at = ?, version = version + 1
		WHERE workspace_id = ? AND id = ? AND deleted_at IS NULL`,
		now, now, workspaceID, id,
	)
	if err != nil {
		return fmt.Errorf("删除短链接失败: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取短链接删除行数失败: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// toDomain 将数据库行转换为域名业务模型
func toDomain(row domainRow) *Domain {
	domain := &Domain{
		ID:        row.ID,
		Hostname:  row.Hostname,
		Status:    row.Status,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
	if row.WorkspaceID.Valid {
		workspaceID := row.WorkspaceID.String
		domain.WorkspaceID = &workspaceID
	}
	return domain
}
