package apikey

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

// MySQLRepository 使用 MySQL 保存 API Key 元数据和摘要
type MySQLRepository struct {
	db *sqlx.DB
}

// NewMySQLRepository 创建 API Key 仓库
func NewMySQLRepository(db *sqlx.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

// Create 保存新的 API Key 摘要
func (r *MySQLRepository) Create(ctx context.Context, key *Key) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO api_keys (
			id, workspace_id, created_by_user_id, name, key_prefix, secret_hash,
			scopes, last_used_at, expires_at, revoked_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		key.ID, key.WorkspaceID, key.CreatedByUserID, key.Name, key.KeyPrefix,
		key.SecretHash, key.Scopes, key.LastUsedAt, key.ExpiresAt, key.RevokedAt,
		key.CreatedAt,
	)
	if err != nil {
		var mysqlErr *driver.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return ErrPrefixConflict
		}
		return fmt.Errorf("保存 API Key 失败: %w", err)
	}
	return nil
}

// ListByWorkspace 返回工作空间 API Key 元数据
func (r *MySQLRepository) ListByWorkspace(ctx context.Context, workspaceID string) ([]Key, error) {
	keys := make([]Key, 0)
	if err := r.db.SelectContext(ctx, &keys, `
		SELECT id, workspace_id, created_by_user_id, name, key_prefix,
			secret_hash, scopes, last_used_at, expires_at, revoked_at, created_at
		FROM api_keys
		WHERE workspace_id = ?
		ORDER BY created_at DESC, id DESC`, workspaceID); err != nil {
		return nil, fmt.Errorf("查询 API Key 列表失败: %w", err)
	}
	return keys, nil
}

// FindByPrefix 按公开前缀查询 API Key 候选
func (r *MySQLRepository) FindByPrefix(ctx context.Context, prefix string) (*Key, error) {
	var key Key
	err := r.db.GetContext(ctx, &key, `
		SELECT id, workspace_id, created_by_user_id, name, key_prefix,
			secret_hash, scopes, last_used_at, expires_at, revoked_at, created_at
		FROM api_keys
		WHERE key_prefix = ?
		LIMIT 1`, prefix)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询 API Key 失败: %w", err)
	}
	return &key, nil
}

// Revoke 撤销当前工作空间的 API Key
func (r *MySQLRepository) Revoke(
	ctx context.Context,
	workspaceID, id string,
	now time.Time,
) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE api_keys SET revoked_at = ?
		WHERE workspace_id = ? AND id = ? AND revoked_at IS NULL`,
		now, workspaceID, id,
	)
	if err != nil {
		return fmt.Errorf("撤销 API Key 失败: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取 API Key 撤销行数失败: %w", err)
	}
	if affected == 1 {
		return nil
	}
	var exists int
	if err := r.db.GetContext(ctx, &exists, `
		SELECT COUNT(*) FROM api_keys WHERE workspace_id = ? AND id = ?`,
		workspaceID, id,
	); err != nil {
		return fmt.Errorf("查询 API Key 是否存在失败: %w", err)
	}
	if exists == 0 {
		return ErrNotFound
	}
	return nil
}

// TouchLastUsed 限频更新 API Key 最近认证时间
func (r *MySQLRepository) TouchLastUsed(ctx context.Context, id string, now time.Time) error {
	if _, err := r.db.ExecContext(ctx, `
		UPDATE api_keys SET last_used_at = ?
		WHERE id = ?
		  AND (last_used_at IS NULL OR last_used_at < ?)`,
		now, id, now.Add(-time.Minute),
	); err != nil {
		return fmt.Errorf("更新 API Key 最近使用时间失败: %w", err)
	}
	return nil
}
