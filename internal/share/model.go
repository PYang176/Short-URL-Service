package share

import (
	"context"
	"errors"
	"time"
)

const (
	// MinDays 是公开分析分享允许的最短范围
	MinDays = 1
	// MaxDays 是公开分析分享允许的最长范围
	MaxDays = 30
)

var (
	// ErrInvalidInput 表示分享参数无效
	ErrInvalidInput = errors.New("分享参数无效")
	// ErrNotFound 表示分享不存在、已撤销或已过期
	ErrNotFound = errors.New("分享不存在")
)

// Share 是一条公开分析分享记录
type Share struct {
	// ID 是 ULID 分享标识
	ID string `db:"id"`
	// WorkspaceID 是分享所属工作空间
	WorkspaceID string `db:"workspace_id"`
	// LinkID 是分享对应的短链标识
	LinkID string `db:"link_id"`
	// CreatedByUserID 是创建分享的用户标识
	CreatedByUserID string `db:"created_by_user_id"`
	// TokenHash 是分享 Token 的 SHA-256 摘要
	TokenHash []byte `db:"token_hash"`
	// Days 是分享创建时固定的分析时间范围
	Days int `db:"share_days"`
	// ExpiresAt 是分享失效时间
	ExpiresAt time.Time `db:"expires_at"`
	// RevokedAt 是撤销时间，nil 表示未撤销
	RevokedAt *time.Time `db:"revoked_at"`
	// CreatedAt 是创建时间
	CreatedAt time.Time `db:"created_at"`
}

// Repository 定义公开分析分享的持久化能力
type Repository interface {
	// Create 保存分享 Token 摘要和有效期
	Create(ctx context.Context, item *Share) error
	// ListActive 返回短链当前有效的分享记录
	ListActive(ctx context.Context, workspaceID, linkID string, now time.Time) ([]Share, error)
	// Revoke 撤销工作空间内的指定分享
	Revoke(ctx context.Context, workspaceID, linkID, id string, now time.Time) error
	// FindActiveByTokenHash 查询未撤销、未过期的分享
	FindActiveByTokenHash(ctx context.Context, tokenHash []byte, now time.Time) (*Share, error)
}

// PublicReport 是公开页面可展示的脱敏分析结果
type PublicReport struct {
	// Title 是分享短链标题
	Title string `json:"title"`
	// Slug 是短链短码
	Slug string `json:"slug"`
	// ShortURL 是公开短链地址
	ShortURL string `json:"short_url"`
	// Days 是固定分析范围
	Days int `json:"days"`
	// ExpiresAt 是分享失效时间
	ExpiresAt time.Time `json:"expires_at"`
}
