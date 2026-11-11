package identity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

// Repository 定义身份和工作空间持久化所需的最小能力
type Repository interface {
	// CreateIdentity 在同一事务中创建用户、工作空间、成员关系和首个 Refresh Token
	CreateIdentity(ctx context.Context, user *User, workspace *Workspace, token *RefreshToken) error
	// FindUserByEmail 按规范化邮箱查询用户
	FindUserByEmail(ctx context.Context, email string) (*User, error)
	// FindUserByID 按 ULID 查询用户
	FindUserByID(ctx context.Context, id string) (*User, error)
	// FindWorkspaceForUser 查询用户当前使用的默认工作空间
	FindWorkspaceForUser(ctx context.Context, userID string) (*Workspace, error)
	// UpdateDisplayName 更新用户展示名称
	UpdateDisplayName(ctx context.Context, userID, displayName string, now time.Time) error
	// CreateRefreshToken 保存新的 Refresh Token 摘要
	CreateRefreshToken(ctx context.Context, token *RefreshToken) error
	// FindRefreshTokenByHash 按摘要查询 Refresh Token
	FindRefreshTokenByHash(ctx context.Context, hash []byte) (*RefreshToken, error)
	// RotateRefreshToken 原子撤销旧 Token 并保存轮换后的新 Token
	RotateRefreshToken(ctx context.Context, oldID string, replacement *RefreshToken, now time.Time) error
	// RevokeTokenFamily 撤销同一登录链上的全部 Token
	RevokeTokenFamily(ctx context.Context, familyID string, now time.Time) error
	// RevokeRefreshToken 按摘要撤销单个 Refresh Token
	RevokeRefreshToken(ctx context.Context, hash []byte, now time.Time) error
}

// MySQLRepository 使用 MySQL 持久化身份和工作空间数据
type MySQLRepository struct {
	db *sqlx.DB
}

// NewMySQLRepository 创建身份持久化仓库
func NewMySQLRepository(db *sqlx.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

// CreateIdentity 在同一事务中创建用户、工作空间、所有者成员关系和首个 Refresh Token
func (r *MySQLRepository) CreateIdentity(ctx context.Context, user *User, workspace *Workspace, token *RefreshToken) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开启身份事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, display_name, role, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		user.ID, user.Email, user.PasswordHash, user.DisplayName, user.Role, user.Status, user.CreatedAt, user.UpdatedAt,
	); err != nil {
		if isDuplicateKey(err) {
			return ErrEmailExists
		}
		return fmt.Errorf("创建用户失败: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO workspaces (id, name, slug, owner_user_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		workspace.ID, workspace.Name, workspace.Slug, workspace.OwnerUserID, workspace.CreatedAt, workspace.UpdatedAt,
	); err != nil {
		return fmt.Errorf("创建工作空间失败: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO workspace_members (workspace_id, user_id, role, created_at)
		VALUES (?, ?, ?, ?)`,
		workspace.ID, user.ID, MemberRoleOwner, user.CreatedAt,
	); err != nil {
		return fmt.Errorf("创建工作空间成员关系失败: %w", err)
	}

	if err := insertRefreshToken(ctx, tx, token); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交身份事务失败: %w", err)
	}
	return nil
}

// FindUserByEmail 按规范化邮箱查询用户
func (r *MySQLRepository) FindUserByEmail(ctx context.Context, email string) (*User, error) {
	var user User
	err := r.db.GetContext(ctx, &user, `
		SELECT id, email, password_hash, display_name, role, status, created_at, updated_at
		FROM users WHERE email = ? LIMIT 1`, email)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("按邮箱查询用户失败: %w", err)
	}
	return &user, nil
}

// FindUserByID 按 ULID 查询用户
func (r *MySQLRepository) FindUserByID(ctx context.Context, id string) (*User, error) {
	var user User
	err := r.db.GetContext(ctx, &user, `
		SELECT id, email, password_hash, display_name, role, status, created_at, updated_at
		FROM users WHERE id = ? LIMIT 1`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("按 ID 查询用户失败: %w", err)
	}
	return &user, nil
}

// FindWorkspaceForUser 按成员创建时间返回用户的默认工作空间
func (r *MySQLRepository) FindWorkspaceForUser(ctx context.Context, userID string) (*Workspace, error) {
	var workspace Workspace
	err := r.db.GetContext(ctx, &workspace, `
		SELECT w.id, w.name, w.slug, w.owner_user_id, w.created_at, w.updated_at
		FROM workspaces w
		INNER JOIN workspace_members wm ON wm.workspace_id = w.id
		WHERE wm.user_id = ?
		ORDER BY wm.created_at ASC
		LIMIT 1`, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询用户工作空间失败: %w", err)
	}
	return &workspace, nil
}

// UpdateDisplayName 更新用户公开展示名称并刷新修改时间
func (r *MySQLRepository) UpdateDisplayName(ctx context.Context, userID, displayName string, now time.Time) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE users SET display_name = ?, updated_at = ? WHERE id = ?`,
		displayName, now, userID,
	)
	if err != nil {
		return fmt.Errorf("更新展示名称失败: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取展示名称更新行数失败: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateRefreshToken 保存新的 Refresh Token 摘要
func (r *MySQLRepository) CreateRefreshToken(ctx context.Context, token *RefreshToken) error {
	if err := insertRefreshToken(ctx, r.db, token); err != nil {
		return err
	}
	return nil
}

// FindRefreshTokenByHash 按 SHA-256 摘要查询 Refresh Token
func (r *MySQLRepository) FindRefreshTokenByHash(ctx context.Context, hash []byte) (*RefreshToken, error) {
	var token RefreshToken
	err := r.db.GetContext(ctx, &token, `
		SELECT id, user_id, token_hash, family_id, expires_at, revoked_at, replaced_by, created_at
		FROM refresh_tokens WHERE token_hash = ? LIMIT 1`, hash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询 Refresh Token 失败: %w", err)
	}
	return &token, nil
}

// RotateRefreshToken 在事务中保存新 Token，并以条件更新抢占旧 Token 的撤销权
func (r *MySQLRepository) RotateRefreshToken(ctx context.Context, oldID string, replacement *RefreshToken, now time.Time) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开启 Refresh Token 轮换事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := insertRefreshToken(ctx, tx, replacement); err != nil {
		return err
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = ?, replaced_by = ?
		WHERE id = ? AND revoked_at IS NULL`,
		now, replacement.ID, oldID,
	)
	if err != nil {
		return fmt.Errorf("撤销旧 Refresh Token 失败: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取 Refresh Token 轮换行数失败: %w", err)
	}
	if affected != 1 {
		return ErrTokenReuse
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交 Refresh Token 轮换事务失败: %w", err)
	}
	return nil
}

// RevokeTokenFamily 撤销同一登录轮换链中所有尚未撤销的 Token
func (r *MySQLRepository) RevokeTokenFamily(ctx context.Context, familyID string, now time.Time) error {
	if _, err := r.db.ExecContext(ctx, `
		UPDATE refresh_tokens SET revoked_at = ?
		WHERE family_id = ? AND revoked_at IS NULL`, now, familyID); err != nil {
		return fmt.Errorf("撤销 Refresh Token Family 失败: %w", err)
	}
	return nil
}

// RevokeRefreshToken 按摘要撤销单个未撤销 Token
func (r *MySQLRepository) RevokeRefreshToken(ctx context.Context, hash []byte, now time.Time) error {
	if _, err := r.db.ExecContext(ctx, `
		UPDATE refresh_tokens SET revoked_at = ?
		WHERE token_hash = ? AND revoked_at IS NULL`, now, hash); err != nil {
		return fmt.Errorf("撤销 Refresh Token 失败: %w", err)
	}
	return nil
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// insertRefreshToken 使用给定执行器插入 Refresh Token，供普通写入和事务复用
func insertRefreshToken(ctx context.Context, executor execer, token *RefreshToken) error {
	if _, err := executor.ExecContext(ctx, `
		INSERT INTO refresh_tokens
			(id, user_id, token_hash, family_id, expires_at, revoked_at, replaced_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		token.ID, token.UserID, token.TokenHash, token.FamilyID,
		token.ExpiresAt, token.RevokedAt, token.ReplacedBy, token.CreatedAt,
	); err != nil {
		return fmt.Errorf("保存 Refresh Token 失败: %w", err)
	}
	return nil
}

// isDuplicateKey 判断 MySQL 是否返回唯一键冲突
func isDuplicateKey(err error) bool {
	var mysqlErr *driver.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}
