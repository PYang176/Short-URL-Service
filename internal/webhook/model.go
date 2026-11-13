package webhook

import (
	"database/sql"
	"time"
)

const (
	// EndpointStatusActive 表示端点可以接收新事件
	EndpointStatusActive uint8 = 1
	// EndpointStatusDisabled 表示端点已停止投递
	EndpointStatusDisabled uint8 = 2

	// DeliveryStatusPending 表示等待投递或重试
	DeliveryStatusPending uint8 = 1
	// DeliveryStatusDelivering 表示 Worker 已领取
	DeliveryStatusDelivering uint8 = 2
	// DeliveryStatusSuccess 表示目标返回 2xx
	DeliveryStatusSuccess uint8 = 3
	// DeliveryStatusDead 表示达到最大重试次数
	DeliveryStatusDead uint8 = 4
)

// Endpoint 是工作空间 Webhook 回调配置
type Endpoint struct {
	// ID 是 ULID Webhook 标识
	ID string `db:"id" json:"id"`
	// WorkspaceID 是所属工作空间 ULID
	WorkspaceID string `db:"workspace_id" json:"-"`
	// Name 是用户可读名称
	Name string `db:"name" json:"name"`
	// URL 是回调地址
	URL string `db:"url" json:"url"`
	// SecretCiphertext 是 AES-GCM 加密后的签名 Secret
	SecretCiphertext []byte `db:"secret_ciphertext" json:"-"`
	// Status 是 EndpointStatusActive 或 EndpointStatusDisabled
	Status uint8 `db:"status" json:"status"`
	// FailureCount 是触发停用前的失败次数
	FailureCount int `db:"failure_count" json:"failure_count"`
	// DisabledAt 是自动或手动停用的 UTC 时间
	DisabledAt *time.Time `db:"disabled_at" json:"disabled_at"`
	// DeletedAt 是软删除 UTC 时间
	DeletedAt *time.Time `db:"deleted_at" json:"deleted_at"`
	// CreatedAt 是创建 UTC 时间
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	// UpdatedAt 是最近修改 UTC 时间
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// Delivery 是 Outbox 中的一次 Webhook 投递
type Delivery struct {
	// ID 是自增 Outbox 标识
	ID uint64 `db:"id" json:"id"`
	// WorkspaceID 是所属工作空间 ULID
	WorkspaceID string `db:"workspace_id" json:"-"`
	// WebhookID 是目标端点 ULID
	WebhookID string `db:"webhook_id" json:"webhook_id"`
	// EventID 是领域事件 ULID
	EventID string `db:"event_id" json:"event_id"`
	// EventType 是 link.created 等事件类型
	EventType string `db:"event_type" json:"event_type"`
	// Payload 是发送给目标的 JSON
	Payload []byte `db:"payload" json:"-"`
	// Status 是投递状态
	Status uint8 `db:"status" json:"status"`
	// AttemptCount 是已尝试次数
	AttemptCount int `db:"attempt_count" json:"attempt_count"`
	// NextAttemptAt 是下一次允许投递的 UTC 时间
	NextAttemptAt time.Time `db:"next_attempt_at" json:"next_attempt_at"`
	// LockedUntil 是 Worker 领取租约到期时间
	LockedUntil *time.Time `db:"locked_until" json:"locked_until"`
	// ResponseStatus 是最近 HTTP 状态码
	ResponseStatus sql.NullInt64 `db:"response_status" json:"response_status"`
	// LastError 是最近一次失败原因
	LastError string `db:"last_error" json:"last_error"`
	// DeliveredAt 是成功投递 UTC 时间
	DeliveredAt *time.Time `db:"delivered_at" json:"delivered_at"`
	// CreatedAt 是创建 UTC 时间
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	// UpdatedAt 是最近修改 UTC 时间
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// DeliveryJob 是 Worker 投递所需的端点和 Outbox 数据
type DeliveryJob struct {
	// Delivery 是 Outbox 记录
	Delivery
	// EndpointURL 是当前回调地址
	EndpointURL string `db:"endpoint_url"`
	// SecretCiphertext 是目标端点签名 Secret 密文
	SecretCiphertext []byte `db:"secret_ciphertext"`
}

// EndpointCreateResult 是只返回一次签名 Secret 的创建结果
type EndpointCreateResult struct {
	// Endpoint 是可公开的端点元数据
	Endpoint Endpoint
	// Secret 是仅创建响应返回的签名 Secret
	Secret string
}
