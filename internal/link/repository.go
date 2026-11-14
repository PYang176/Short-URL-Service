package link

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

// Repository 定义短链接持久化所需的最小能力
type Repository interface {
	// EnsureDefaultDomain 查询或创建平台默认域名
	EnsureDefaultDomain(ctx context.Context, hostname string, now time.Time) (*Domain, error)
	// Create 在同一事务中保存新短链接和可选 Webhook 事件
	Create(ctx context.Context, link *Link, event *OutboxEvent) error
	// FindByID 按工作空间和链接 ID 查询未删除链接
	FindByID(ctx context.Context, workspaceID, id string) (*Link, error)
	// FindByDomainSlug 按域名和短码查询未删除链接
	FindByDomainSlug(ctx context.Context, domainID, slug string) (*Link, error)
	// List 按工作空间、筛选条件和游标倒序查询链接
	List(
		ctx context.Context,
		workspaceID string,
		limit int,
		beforeCreatedAt *time.Time,
		beforeID string,
		filter ListFilter,
	) ([]Link, error)
	// Update 在同一事务中保存可编辑字段和可选 Webhook 事件
	Update(ctx context.Context, link *Link, event *OutboxEvent) error
	// SoftDelete 在同一事务中软删除链接和写入可选 Webhook 事件
	SoftDelete(
		ctx context.Context,
		workspaceID, id string,
		now time.Time,
		event *OutboxEvent,
	) error
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

// Create 在同一事务中保存新短链接和可选 Webhook 事件
func (r *MySQLRepository) Create(ctx context.Context, link *Link, event *OutboxEvent) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始创建短链接事务失败: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO links (
			id, workspace_id, domain_id, slug, destination_url, destination_hash,
			title, notes, idempotency_key, status, redirect_code, starts_at, expires_at,
			human_clicks, version, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		link.ID, link.WorkspaceID, link.DomainID, link.Slug, link.DestinationURL,
		link.DestinationHash, link.Title, link.Notes, nullString(link.IdempotencyKey),
		link.Status, link.RedirectCode, link.StartsAt, link.ExpiresAt,
		link.HumanClicks, link.Version,
		link.CreatedAt, link.UpdatedAt,
	)
	if err != nil {
		if isDuplicateKey(err) {
			if link.IdempotencyKey != "" {
				existing, findErr := findByIdempotencyKey(ctx, tx, link.WorkspaceID, link.IdempotencyKey)
				if findErr != nil {
					return findErr
				}
				if existing != nil {
					*link = *existing
					return nil
				}
			}
			return ErrSlugTaken
		}
		return fmt.Errorf("创建短链接失败: %w", err)
	}
	if err := enqueueOutbox(ctx, tx, link.WorkspaceID, event, link.CreatedAt); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交创建短链接事务失败: %w", err)
	}
	return nil
}

// FindByID 按工作空间和链接 ID 查询未删除链接
func (r *MySQLRepository) FindByID(ctx context.Context, workspaceID, id string) (*Link, error) {
	var result Link
	err := r.db.GetContext(ctx, &result, `
		SELECT l.id, l.workspace_id, l.domain_id, d.hostname AS domain_hostname,
			l.slug, l.destination_url, l.destination_hash, l.title, l.notes,
			COALESCE(l.idempotency_key, '') AS idempotency_key,
			l.status, l.redirect_code, l.starts_at, l.expires_at,
			l.human_clicks, l.version, l.created_at, l.updated_at, l.deleted_at
		FROM links AS l
		INNER JOIN domains AS d ON d.id = l.domain_id
		WHERE l.workspace_id = ? AND l.id = ? AND l.deleted_at IS NULL
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
		SELECT l.id, l.workspace_id, l.domain_id, d.hostname AS domain_hostname,
			l.slug, l.destination_url, l.destination_hash, l.title, l.notes,
			COALESCE(l.idempotency_key, '') AS idempotency_key,
			l.status, l.redirect_code, l.starts_at, l.expires_at,
			l.human_clicks, l.version, l.created_at, l.updated_at, l.deleted_at
		FROM links AS l
		INNER JOIN domains AS d ON d.id = l.domain_id
		WHERE l.domain_id = ? AND l.slug = ? AND l.deleted_at IS NULL
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
	filter ListFilter,
) ([]Link, error) {
	query := `
		SELECT l.id, l.workspace_id, l.domain_id, d.hostname AS domain_hostname,
			l.slug, l.destination_url, l.destination_hash, l.title, l.notes,
			COALESCE(l.idempotency_key, '') AS idempotency_key,
			l.status, l.redirect_code, l.starts_at, l.expires_at,
			l.human_clicks, l.version, l.created_at, l.updated_at, l.deleted_at
		FROM links AS l
		INNER JOIN domains AS d ON d.id = l.domain_id
		WHERE l.workspace_id = ? AND l.deleted_at IS NULL`
	args := []any{workspaceID}
	if filter.Query != "" {
		query += `
		  AND (
			l.title LIKE ? ESCAPE '!'
			OR LOWER(l.slug) LIKE ? ESCAPE '!'
			OR l.destination_url LIKE ? ESCAPE '!'
		  )`
		pattern := "%" + escapeLikePattern(strings.ToLower(filter.Query)) + "%"
		args = append(args, pattern, pattern, pattern)
	}
	if filter.Status != nil {
		query += " AND l.status = ?"
		args = append(args, *filter.Status)
	}
	if beforeCreatedAt != nil && beforeID != "" {
		query += " AND (l.created_at, l.id) < (?, ?)"
		args = append(args, *beforeCreatedAt, beforeID)
	}
	query += " ORDER BY l.created_at DESC, l.id DESC LIMIT ?"
	args = append(args, limit)

	links := make([]Link, 0, limit)
	if err := r.db.SelectContext(ctx, &links, query, args...); err != nil {
		return nil, fmt.Errorf("查询短链接列表失败: %w", err)
	}
	return links, nil
}

// escapeLikePattern 转义 LIKE 通配符，保证搜索词按普通文本匹配。
func escapeLikePattern(value string) string {
	replacer := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_")
	return replacer.Replace(value)
}

// Update 在同一事务中保存可编辑字段和可选 Webhook 事件
func (r *MySQLRepository) Update(ctx context.Context, link *Link, event *OutboxEvent) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始更新短链接事务失败: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	result, err := tx.ExecContext(ctx, `
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
	if err := enqueueOutbox(ctx, tx, link.WorkspaceID, event, link.UpdatedAt); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交更新短链接事务失败: %w", err)
	}
	return nil
}

// SoftDelete 在同一事务中软删除链接和写入可选 Webhook 事件
func (r *MySQLRepository) SoftDelete(
	ctx context.Context,
	workspaceID, id string,
	now time.Time,
	event *OutboxEvent,
) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始删除短链接事务失败: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	result, err := tx.ExecContext(ctx, `
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
	if err := enqueueOutbox(ctx, tx, workspaceID, event, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交删除短链接事务失败: %w", err)
	}
	return nil
}

// enqueueOutbox 将事件展开到当前工作空间所有启用端点
func enqueueOutbox(
	ctx context.Context,
	tx *sqlx.Tx,
	workspaceID string,
	event *OutboxEvent,
	now time.Time,
) error {
	if event == nil {
		return nil
	}
	if event.ID == "" || event.Type == "" || len(event.Payload) == 0 {
		return fmt.Errorf("webhook Outbox 事件字段不完整")
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO webhook_deliveries (
			workspace_id, webhook_id, event_id, event_type, payload,
			status, attempt_count, next_attempt_at, created_at, updated_at
		)
		SELECT ?, id, ?, ?, ?, 1, 0, ?, ?, ?
		FROM webhook_endpoints
		WHERE workspace_id = ? AND status = 1 AND deleted_at IS NULL`,
		workspaceID, event.ID, event.Type, string(event.Payload),
		now, now, now, workspaceID,
	); err != nil {
		return fmt.Errorf("写入 Webhook Outbox 失败: %w", err)
	}
	return nil
}

// nullString 将空字符串转换为 SQL NULL，避免空幂等键触发唯一约束冲突。
func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// findByIdempotencyKey 在事务内查询已提交的同一幂等链接。
func findByIdempotencyKey(
	ctx context.Context,
	tx *sqlx.Tx,
	workspaceID, key string,
) (*Link, error) {
	var result Link
	err := tx.GetContext(ctx, &result, `
		SELECT l.id, l.workspace_id, l.domain_id, d.hostname AS domain_hostname,
			l.slug, l.destination_url, l.destination_hash, l.title, l.notes,
			COALESCE(l.idempotency_key, '') AS idempotency_key,
			l.status, l.redirect_code, l.starts_at, l.expires_at,
			l.human_clicks, l.version, l.created_at, l.updated_at, l.deleted_at
		FROM links AS l
		INNER JOIN domains AS d ON d.id = l.domain_id
		WHERE l.workspace_id = ? AND l.idempotency_key = ?
		LIMIT 1`, workspaceID, key)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("按幂等键查询短链接失败: %w", err)
	}
	return &result, nil
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
