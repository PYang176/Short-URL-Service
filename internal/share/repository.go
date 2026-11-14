package share

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// MySQLRepository 使用 MySQL 保存公开分析分享
type MySQLRepository struct {
	db *sqlx.DB
}

// NewMySQLRepository 创建公开分析分享仓库
func NewMySQLRepository(db *sqlx.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

// Create 保存分享 Token 摘要和有效期
func (r *MySQLRepository) Create(ctx context.Context, item *Share) error {
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO analytics_shares (
			id, workspace_id, link_id, created_by_user_id, token_hash,
			share_days, expires_at, revoked_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.WorkspaceID, item.LinkID, item.CreatedByUserID,
		item.TokenHash, item.Days, item.ExpiresAt, item.RevokedAt, item.CreatedAt,
	); err != nil {
		return fmt.Errorf("保存公开分析分享失败: %w", err)
	}
	return nil
}

// ListActive 返回短链当前有效的分享记录
func (r *MySQLRepository) ListActive(
	ctx context.Context,
	workspaceID, linkID string,
	now time.Time,
) ([]Share, error) {
	items := make([]Share, 0)
	if err := r.db.SelectContext(ctx, &items, `
		SELECT id, workspace_id, link_id, created_by_user_id, token_hash,
			share_days, expires_at, revoked_at, created_at
		FROM analytics_shares
		WHERE workspace_id = ? AND link_id = ?
		  AND revoked_at IS NULL AND expires_at > ?
		ORDER BY created_at DESC, id DESC`, workspaceID, linkID, now); err != nil {
		return nil, fmt.Errorf("查询公开分析分享失败: %w", err)
	}
	return items, nil
}

// Revoke 撤销工作空间内的指定分享
func (r *MySQLRepository) Revoke(
	ctx context.Context,
	workspaceID, linkID, id string,
	now time.Time,
) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE analytics_shares
		SET revoked_at = ?
		WHERE workspace_id = ? AND link_id = ? AND id = ? AND revoked_at IS NULL`,
		now, workspaceID, linkID, id,
	)
	if err != nil {
		return fmt.Errorf("撤销公开分析分享失败: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取公开分析分享撤销行数失败: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// FindActiveByTokenHash 查询未撤销、未过期的分享
func (r *MySQLRepository) FindActiveByTokenHash(
	ctx context.Context,
	tokenHash []byte,
	now time.Time,
) (*Share, error) {
	var item Share
	err := r.db.GetContext(ctx, &item, `
		SELECT id, workspace_id, link_id, created_by_user_id, token_hash,
			share_days, expires_at, revoked_at, created_at
		FROM analytics_shares
		WHERE token_hash = ? AND revoked_at IS NULL AND expires_at > ?
		LIMIT 1`, tokenHash, now)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询公开分析分享失败: %w", err)
	}
	return &item, nil
}
