package trial

import "time"

const (
	// WorkspaceKindTrial 表示不绑定登录用户的游客工作空间
	WorkspaceKindTrial uint8 = 2
	// CookieName 是浏览器保存游客原始 Token 的 Cookie 名称
	CookieName = "shorturl_trial"
)

// Session 是游客 Cookie 对应的服务端会话状态
type Session struct {
	// ID 是 ULID 游客会话标识
	ID string `db:"id"`
	// WorkspaceID 是会话创建的游客工作空间 ULID
	WorkspaceID string `db:"workspace_id"`
	// TokenHash 是 Cookie Token 的 SHA-256 摘要
	TokenHash []byte `db:"token_hash"`
	// LinkCount 是当前会话已经创建的链接数量
	LinkCount uint16 `db:"link_count"`
	// ExpiresAt 是会话和名下临时链接共同失效的 UTC 时间
	ExpiresAt time.Time `db:"expires_at"`
	// CreatedAt 是会话创建的 UTC 时间
	CreatedAt time.Time `db:"created_at"`
	// UpdatedAt 是会话最近修改的 UTC 时间
	UpdatedAt time.Time `db:"updated_at"`
}
