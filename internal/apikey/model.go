package apikey

import "time"

const (
	// ScopeLinksRead 允许读取短链接
	ScopeLinksRead = "links:read"
	// ScopeLinksWrite 允许创建和修改短链接
	ScopeLinksWrite = "links:write"
	// ScopeAnalyticsRead 允许读取点击分析
	ScopeAnalyticsRead = "analytics:read"
)

// AllScopes 是 API Key 当前支持的稳定作用域顺序
var AllScopes = []string{
	ScopeLinksRead,
	ScopeLinksWrite,
	ScopeAnalyticsRead,
}

// Key 是可公开展示的 API Key 元数据
type Key struct {
	// ID 是 ULID API Key 记录标识
	ID string `db:"id" json:"id"`
	// WorkspaceID 是 API Key 所属工作空间 ULID
	WorkspaceID string `db:"workspace_id" json:"-"`
	// CreatedByUserID 是创建者用户 ULID
	CreatedByUserID string `db:"created_by_user_id" json:"-"`
	// Name 是用户设置的 API Key 名称
	Name string `db:"name" json:"name"`
	// KeyPrefix 是可展示和索引的前缀，不包含 Secret
	KeyPrefix string `db:"key_prefix" json:"prefix"`
	// SecretHash 是完整原始 Key 的 SHA-256 摘要
	SecretHash []byte `db:"secret_hash" json:"-"`
	// Scopes 是逗号分隔的权限作用域
	Scopes string `db:"scopes" json:"-"`
	// LastUsedAt 是最近一次认证成功的 UTC 时间
	LastUsedAt *time.Time `db:"last_used_at" json:"last_used_at"`
	// ExpiresAt 是可选失效 UTC 时间
	ExpiresAt *time.Time `db:"expires_at" json:"expires_at"`
	// RevokedAt 是撤销 UTC 时间，nil 表示有效
	RevokedAt *time.Time `db:"revoked_at" json:"revoked_at"`
	// CreatedAt 是创建 UTC 时间
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// Principal 是 API Key 认证后的最小身份上下文
type Principal struct {
	// ID 是 API Key 记录 ULID
	ID string
	// WorkspaceID 是 API Key 所属工作空间 ULID
	WorkspaceID string
	// Scopes 是授予当前请求的稳定作用域集合
	Scopes []string
}

// CreateResult 是只返回一次明文 Key 的创建结果
type CreateResult struct {
	// Key 是可公开的 API Key 元数据
	Key Key
	// Plaintext 是仅创建响应返回的完整 API Key
	Plaintext string
}
