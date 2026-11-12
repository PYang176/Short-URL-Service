package analytics

import "time"

// Event 是一条已经脱敏的点击事件
type Event struct {
	// EventID 是 ULID 事件标识，也是消费幂等键
	EventID string `db:"event_id"`
	// WorkspaceID 是链接所属的 ULID 工作空间标识
	WorkspaceID string `db:"workspace_id"`
	// LinkID 是 ULID 链接标识
	LinkID string `db:"link_id"`
	// DomainID 是 ULID 域名标识
	DomainID string `db:"domain_id"`
	// Slug 是访问时的短码快照
	Slug string `db:"slug"`
	// ClickedAt 是点击发生时的 UTC 时间
	ClickedAt time.Time `db:"clicked_at"`
	// VisitorHash 是按日加盐的访客 HMAC
	VisitorHash []byte `db:"visitor_hash"`
	// Country 是 ISO 3166-1 alpha-2 国家代码
	Country string `db:"country"`
	// Device 是 mobile、desktop 或 tablet
	Device string `db:"device"`
	// OS 是识别出的操作系统
	OS string `db:"os"`
	// Browser 是识别出的浏览器
	Browser string `db:"browser"`
	// Language 是主浏览器语言
	Language string `db:"language"`
	// Referrer 是来源页地址
	Referrer string `db:"referrer"`
	// UTMSource 是 utm_source 参数
	UTMSource string `db:"utm_source"`
	// UTMMedium 是 utm_medium 参数
	UTMMedium string `db:"utm_medium"`
	// UTMCampaign 是 utm_campaign 参数
	UTMCampaign string `db:"utm_campaign"`
	// UTMTerm 是 utm_term 参数
	UTMTerm string `db:"utm_term"`
	// UTMContent 是 utm_content 参数
	UTMContent string `db:"utm_content"`
	// IsBot 表示事件是否来自已识别机器人
	IsBot bool `db:"is_bot"`
}
