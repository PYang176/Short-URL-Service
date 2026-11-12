package domain

import "time"

const (
	// StatusPending 表示域名等待用户配置 DNS
	StatusPending uint8 = 1
	// StatusVerifying 表示域名正在验证
	StatusVerifying uint8 = 2
	// StatusVerified 表示域名可以承载短链
	StatusVerified uint8 = 3
	// StatusFailed 表示最近一次验证失败
	StatusFailed uint8 = 4
	// StatusDisabled 表示域名已停用
	StatusDisabled uint8 = 5
)

// Domain 是工作空间自定义域名
type Domain struct {
	// ID 是 ULID 域名标识
	ID string `db:"id"`
	// WorkspaceID 是 ULID 工作空间标识
	WorkspaceID string `db:"workspace_id"`
	// Hostname 是规范化后的小写域名
	Hostname string `db:"hostname"`
	// Status 是验证状态
	Status uint8 `db:"status"`
	// VerificationToken 是 TXT 记录中的验证值
	VerificationToken string `db:"verification_token"`
	// ExpectedCNAME 是用户域名需要指向的平台 CNAME
	ExpectedCNAME string `db:"expected_cname"`
	// VerificationAttempts 是累计验证次数
	VerificationAttempts int `db:"verification_attempts"`
	// LastVerifiedAt 是最近成功验证时间
	LastVerifiedAt *time.Time `db:"last_verified_at"`
	// NextVerifyAt 是下一次允许自动验证的时间
	NextVerifyAt *time.Time `db:"next_verify_at"`
	// CreatedAt 是域名创建的 UTC 时间
	CreatedAt time.Time `db:"created_at"`
	// UpdatedAt 是域名最近修改的 UTC 时间
	UpdatedAt time.Time `db:"updated_at"`
	// DeletedAt 是软删除时间，nil 表示未删除
	DeletedAt *time.Time `db:"deleted_at"`
}
