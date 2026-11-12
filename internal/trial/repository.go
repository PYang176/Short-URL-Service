package trial

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

// MySQLRepository 使用 MySQL 持久化游客会话
type MySQLRepository struct {
	db *sqlx.DB
}

// NewMySQLRepository 创建游客会话仓库
func NewMySQLRepository(db *sqlx.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

// Create 在同一事务中创建游客工作空间和会话
func (r *MySQLRepository) Create(ctx context.Context, session *Session, name, slug string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开启游客会话事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO workspaces (
			id, kind, name, slug, owner_user_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, NULL, ?, ?)`,
		session.WorkspaceID, WorkspaceKindTrial, name, slug,
		session.CreatedAt, session.UpdatedAt,
	); err != nil {
		return fmt.Errorf("创建游客工作空间失败: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO trial_sessions (
			id, workspace_id, token_hash, link_count,
			expires_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		session.ID, session.WorkspaceID, session.TokenHash, session.LinkCount,
		session.ExpiresAt, session.CreatedAt, session.UpdatedAt,
	); err != nil {
		return fmt.Errorf("创建游客会话失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交游客会话事务失败: %w", err)
	}
	return nil
}

// FindByTokenHash 按 Cookie Token 摘要查询游客会话
func (r *MySQLRepository) FindByTokenHash(ctx context.Context, tokenHash []byte) (*Session, error) {
	var session Session
	err := r.db.GetContext(ctx, &session, `
		SELECT id, workspace_id, token_hash, link_count, expires_at, created_at, updated_at
		FROM trial_sessions
		WHERE token_hash = ?
		LIMIT 1`, tokenHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询游客会话失败: %w", err)
	}
	return &session, nil
}

// ReserveLinkSlot 原子占用一个链接配额
func (r *MySQLRepository) ReserveLinkSlot(
	ctx context.Context,
	sessionID string,
	maxLinks int,
	now time.Time,
) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE trial_sessions
		SET link_count = link_count + 1, updated_at = ?
		WHERE id = ? AND expires_at > ? AND link_count < ?`,
		now, sessionID, now, maxLinks,
	)
	if err != nil {
		return fmt.Errorf("占用游客链接配额失败: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取游客链接配额更新行数失败: %w", err)
	}
	if affected == 0 {
		return ErrLinkLimitReached
	}
	return nil
}

// ReleaseLinkSlot 在链接创建失败时归还配额
func (r *MySQLRepository) ReleaseLinkSlot(ctx context.Context, sessionID string, now time.Time) error {
	if _, err := r.db.ExecContext(ctx, `
		UPDATE trial_sessions
		SET link_count = IF(link_count > 0, link_count - 1, 0), updated_at = ?
		WHERE id = ?`, now, sessionID); err != nil {
		return fmt.Errorf("归还游客链接配额失败: %w", err)
	}
	return nil
}

// CleanupExpired 分批删除过期游客工作空间及关联数据
func (r *MySQLRepository) CleanupExpired(
	ctx context.Context,
	now time.Time,
	limit int,
) (int, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("开启游客清理事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	workspaceIDs := make([]string, 0, limit)
	if err := tx.SelectContext(ctx, &workspaceIDs, `
		SELECT workspace_id
		FROM trial_sessions
		WHERE expires_at <= ?
		ORDER BY expires_at
		LIMIT ?
		FOR UPDATE SKIP LOCKED`, now, limit); err != nil {
		return 0, fmt.Errorf("查询过期游客会话失败: %w", err)
	}
	if len(workspaceIDs) == 0 {
		if err := tx.Commit(); err != nil {
			return 0, fmt.Errorf("提交空游客清理事务失败: %w", err)
		}
		return 0, nil
	}

	clickQuery, clickArgs, err := sqlx.In(
		`DELETE FROM click_events WHERE workspace_id IN (?)`,
		workspaceIDs,
	)
	if err != nil {
		return 0, fmt.Errorf("构造游客点击清理语句失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(clickQuery), clickArgs...); err != nil {
		return 0, fmt.Errorf("删除游客点击事件失败: %w", err)
	}

	linkQuery, linkArgs, err := sqlx.In(
		`DELETE FROM links WHERE workspace_id IN (?)`,
		workspaceIDs,
	)
	if err != nil {
		return 0, fmt.Errorf("构造游客链接清理语句失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(linkQuery), linkArgs...); err != nil {
		return 0, fmt.Errorf("删除游客链接失败: %w", err)
	}

	sessionQuery, sessionArgs, err := sqlx.In(
		`DELETE FROM trial_sessions WHERE workspace_id IN (?)`,
		workspaceIDs,
	)
	if err != nil {
		return 0, fmt.Errorf("构造游客会话清理语句失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(sessionQuery), sessionArgs...); err != nil {
		return 0, fmt.Errorf("删除游客会话失败: %w", err)
	}

	workspaceQuery, workspaceArgs, err := sqlx.In(
		`DELETE FROM workspaces WHERE kind = ? AND id IN (?)`,
		WorkspaceKindTrial,
		workspaceIDs,
	)
	if err != nil {
		return 0, fmt.Errorf("构造游客工作空间清理语句失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(workspaceQuery), workspaceArgs...); err != nil {
		return 0, fmt.Errorf("删除游客工作空间失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("提交游客清理事务失败: %w", err)
	}
	return len(workspaceIDs), nil
}

// Claim 将游客工作空间中的链接和点击迁移到正式工作空间
func (r *MySQLRepository) Claim(
	ctx context.Context,
	tokenHash []byte,
	targetWorkspaceID string,
	now time.Time,
) (*ClaimResult, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("开启游客认领事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var session Session
	err = tx.GetContext(ctx, &session, `
		SELECT id, workspace_id, token_hash, link_count, expires_at, created_at, updated_at
		FROM trial_sessions
		WHERE token_hash = ?
		FOR UPDATE`, tokenHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("锁定游客会话失败: %w", err)
	}
	if !session.ExpiresAt.After(now) {
		return nil, ErrSessionExpired
	}

	var targetKind uint8
	err = tx.GetContext(ctx, &targetKind, `
		SELECT kind FROM workspaces WHERE id = ? FOR UPDATE`, targetWorkspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrClaimTargetNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("锁定认领目标工作空间失败: %w", err)
	}
	if targetKind != 1 {
		return nil, ErrClaimTargetNotFound
	}

	cacheKeys := make([]ClaimedLink, 0, session.LinkCount)
	if err := tx.SelectContext(ctx, &cacheKeys, `
		SELECT domain_id, slug
		FROM links
		WHERE workspace_id = ?
		FOR UPDATE`, session.WorkspaceID); err != nil {
		return nil, fmt.Errorf("锁定游客链接失败: %w", err)
	}

	linkResult, err := tx.ExecContext(ctx, `
		UPDATE links
		SET workspace_id = ?,
			expires_at = CASE WHEN deleted_at IS NULL THEN NULL ELSE expires_at END,
			version = version + 1,
			updated_at = ?
		WHERE workspace_id = ?`,
		targetWorkspaceID, now, session.WorkspaceID,
	)
	if err != nil {
		var mysqlErr *driver.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return nil, ErrClaimConflict
		}
		return nil, fmt.Errorf("迁移游客链接失败: %w", err)
	}
	linkCount, err := linkResult.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("读取游客链接迁移行数失败: %w", err)
	}

	clickResult, err := tx.ExecContext(ctx, `
		UPDATE click_events SET workspace_id = ? WHERE workspace_id = ?`,
		targetWorkspaceID, session.WorkspaceID,
	)
	if err != nil {
		return nil, fmt.Errorf("迁移游客点击事件失败: %w", err)
	}
	clickCount, err := clickResult.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("读取游客点击迁移行数失败: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM idempotency_keys WHERE workspace_id = ?`, session.WorkspaceID); err != nil {
		return nil, fmt.Errorf("删除游客幂等状态失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM trial_sessions WHERE id = ?`, session.ID); err != nil {
		return nil, fmt.Errorf("删除已认领游客会话失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM workspaces WHERE id = ? AND kind = ?`,
		session.WorkspaceID, WorkspaceKindTrial,
	); err != nil {
		return nil, fmt.Errorf("删除已认领游客工作空间失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("提交游客认领事务失败: %w", err)
	}
	return &ClaimResult{
		Links:     linkCount,
		Clicks:    clickCount,
		CacheKeys: cacheKeys,
	}, nil
}
