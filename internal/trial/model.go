package trial

import "time"

const (
	// WorkspaceKindTrial 表示不绑定登录用户的游客工作空间
	WorkspaceKindTrial uint8 = 2
	// CookieName 是浏览器保存游客原始 Token 的 Cookie 名称
	CookieName = "shorturl_trial"

	// ExpiryMinutes15 表示 15 分钟试用有效期
	ExpiryMinutes15 = 15
	// ExpiryMinutes30 表示 30 分钟试用有效期
	ExpiryMinutes30 = 30
	// ExpiryMinutes60 表示 1 小时试用有效期
	ExpiryMinutes60 = 60
	// ExpiryMinutes720 表示 12 小时试用有效期
	ExpiryMinutes720 = 720
	// ExpiryMinutes1440 表示 24 小时试用有效期
	ExpiryMinutes1440 = 1440
)

var trialExpiryDurations = map[int]time.Duration{
	ExpiryMinutes15:   15 * time.Minute,
	ExpiryMinutes30:   30 * time.Minute,
	ExpiryMinutes60:   time.Hour,
	ExpiryMinutes720:  12 * time.Hour,
	ExpiryMinutes1440: 24 * time.Hour,
}

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

// ClaimedLink 是认领后需要失效旧跳转缓存的链接定位信息
type ClaimedLink struct {
	// DomainID 是链接使用的域名 ULID
	DomainID string `db:"domain_id"`
	// Slug 是链接短码
	Slug string `db:"slug"`
}

// ClaimResult 是游客链接认领的事务结果
type ClaimResult struct {
	// Links 是迁移的链接数量
	Links int64
	// Clicks 是迁移的点击明细数量
	Clicks int64
	// CacheKeys 是需要失效旧缓存的链接
	CacheKeys []ClaimedLink
}
