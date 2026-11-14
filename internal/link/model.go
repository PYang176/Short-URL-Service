package link

import (
	"context"
	"net/url"
	"time"
)

const (
	// StatusActive 表示链接可以正常跳转
	StatusActive uint8 = 1
	// StatusDisabled 表示链接已被用户停用
	StatusDisabled uint8 = 2

	// DomainStatusPending 表示域名等待验证
	DomainStatusPending uint8 = 1
	// DomainStatusVerifying 表示域名正在验证
	DomainStatusVerifying uint8 = 2
	// DomainStatusVerified 表示域名可以承载跳转
	DomainStatusVerified uint8 = 3
	// DomainStatusFailed 表示域名验证失败
	DomainStatusFailed uint8 = 4
	// DomainStatusDisabled 表示域名已停止使用
	DomainStatusDisabled uint8 = 5
)

// Domain 是短链 Host 的权威记录
type Domain struct {
	// ID 是 ULID 域名标识
	ID string
	// WorkspaceID 是可空的 ULID 工作空间标识；空值表示平台默认域名
	WorkspaceID *string
	// Hostname 是规范化后的小写域名
	Hostname string
	// Status 是域名状态，有效值为 DomainStatusPending 至 DomainStatusDisabled
	Status uint8
	// CreatedAt 是域名创建的 UTC 时间
	CreatedAt time.Time
	// UpdatedAt 是域名最近修改的 UTC 时间
	UpdatedAt time.Time
}

// Link 是可跳转的短链接
type Link struct {
	// ID 是 ULID 链接标识
	ID string `db:"id"`
	// WorkspaceID 是链接所属的 ULID 工作空间标识
	WorkspaceID string `db:"workspace_id"`
	// DomainID 是链接所用 ULID 域名标识
	DomainID string `db:"domain_id"`
	// DomainHostname 是域名规范化后的 Host
	DomainHostname string `db:"domain_hostname"`
	// Slug 是大小写敏感的唯一短码
	Slug string `db:"slug"`
	// DestinationURL 是规范化后的目标地址
	DestinationURL string `db:"destination_url"`
	// DestinationHash 是目标地址的 SHA-256 摘要
	DestinationHash []byte `db:"destination_hash"`
	// Title 是用户可编辑的链接标题
	Title string `db:"title"`
	// Notes 是用户可编辑的备注
	Notes string `db:"notes"`
	// IdempotencyKey 是创建该链接时使用的幂等键
	IdempotencyKey string `db:"idempotency_key"`
	// Status 是链接状态，有效值为 StatusActive 或 StatusDisabled
	Status uint8 `db:"status"`
	// RedirectCode 是 301 或 302
	RedirectCode int `db:"redirect_code"`
	// StartsAt 是可选生效时间的 UTC 值
	StartsAt *time.Time `db:"starts_at"`
	// ExpiresAt 是可选过期时间的 UTC 值
	ExpiresAt *time.Time `db:"expires_at"`
	// HumanClicks 是人类点击次数，后续由分析汇总维护
	HumanClicks int64 `db:"human_clicks"`
	// Version 是链接元数据版本，写入缓存时用于识别变更
	Version int `db:"version"`
	// CreatedAt 是链接创建的 UTC 时间
	CreatedAt time.Time `db:"created_at"`
	// UpdatedAt 是链接最近修改的 UTC 时间
	UpdatedAt time.Time `db:"updated_at"`
	// DeletedAt 是软删除的 UTC 时间，nil 表示未删除
	DeletedAt *time.Time `db:"deleted_at"`
}

// CreateInput 是创建短链接所需的业务参数
type CreateInput struct {
	// DestinationURL 是用户输入的原始目标地址
	DestinationURL string
	// DomainID 是可选自定义域名标识，为空时使用平台默认域名
	DomainID string
	// Alias 是可选自定义短码
	Alias string
	// Title 是可选链接标题
	Title string
	// Notes 是可选备注
	Notes string
	// IdempotencyKey 是可选创建幂等键，用于崩溃恢复和重试
	IdempotencyKey string
	// RedirectCode 是 301 或 302
	RedirectCode int
	// StartsAt 是可选生效时间
	StartsAt *time.Time
	// ExpiresAt 是可选过期时间
	ExpiresAt *time.Time
}

// ListFilter 是链接列表的组合筛选条件。
type ListFilter struct {
	// Query 是标题、短码或目标地址的模糊搜索关键词
	Query string
	// Status 为 nil 时不过滤，否则只返回指定状态
	Status *uint8
}

// UpdateInput 是修改短链接的可选字段
type UpdateInput struct {
	// DestinationURL 是可选的新目标地址
	DestinationURL *string
	// Title 是可选的新标题
	Title *string
	// Notes 是可选的新备注
	Notes *string
	// Status 是可选的新状态
	Status *uint8
	// RedirectCode 是可选的新跳转状态码
	RedirectCode *int
	// ExpiresAt 是可选的新过期时间；ClearExpiresAt 为 true 时清除
	ExpiresAt *time.Time
	// ClearExpiresAt 表示删除已有过期时间
	ClearExpiresAt bool
}

// CachedLink 是 Redis 中用于跳转的最小链接快照
type CachedLink struct {
	// ID 是 ULID 链接标识
	ID string `json:"id"`
	// WorkspaceID 是链接所属的 ULID 工作空间标识
	WorkspaceID string `json:"workspace_id"`
	// DestinationURL 是当前跳转目标
	DestinationURL string `json:"destination_url"`
	// Status 是链接状态
	Status uint8 `json:"status"`
	// RedirectCode 是 301 或 302
	RedirectCode int `json:"redirect_code"`
	// StartsAt 是可选生效时间
	StartsAt *time.Time `json:"starts_at,omitempty"`
	// ExpiresAt 是可选过期时间
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	// Version 是缓存对应的链接版本
	Version int `json:"version"`
}

// ResolveResult 是公开跳转解析后的链接元数据
type ResolveResult struct {
	// LinkID 是 ULID 链接标识
	LinkID string
	// WorkspaceID 是链接所属工作空间
	WorkspaceID string
	// DomainID 是链接域名
	DomainID string
	// Slug 是访问的短码
	Slug string
	// DestinationURL 是最终跳转目标
	DestinationURL string
	// RedirectCode 是 301 或 302
	RedirectCode int
}

// ClickContext 是跳转请求交给点击记录器的最小上下文
type ClickContext struct {
	// ResolveResult 是已解析链接的元数据
	ResolveResult
	// IP 是经过代理配置解析后的客户端地址
	IP string
	// UserAgent 是请求的原始 User-Agent
	UserAgent string
	// Referrer 是请求来源页
	Referrer string
	// Language 是 Accept-Language 原文
	Language string
	// Country 是可信代理提供的 ISO 国家代码
	Country string
	// Query 是访问短链时携带的查询参数
	Query url.Values
}

// ClickRecorder 记录一次成功的公开跳转
type ClickRecorder interface {
	// RecordClick 将点击上下文写入异步事件队列
	RecordClick(ctx context.Context, click ClickContext) error
}

// OutboxEvent 是与链接写入同事务提交的 Webhook 领域事件
type OutboxEvent struct {
	// ID 是事件 ULID，同一事件投递到多个端点时保持不变
	ID string
	// Type 是 link.created、link.updated 或 link.deleted
	Type string
	// Payload 是发送给 Webhook 目标的完整 JSON
	Payload []byte
}
