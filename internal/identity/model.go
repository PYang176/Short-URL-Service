package identity

import "time"

const (
	// ContextUserID 是 Gin 上下文中的当前用户 ID 键
	ContextUserID = "user_id"
	// ContextWorkspaceID 是 Gin 上下文中的当前工作空间 ID 键
	ContextWorkspaceID = "workspace_id"
	// ContextMemberRole 是 Gin 上下文中的工作空间角色键
	ContextMemberRole = "member_role"
	// ContextUserRole 是 Gin 上下文中的平台角色键
	ContextUserRole = "user_role"
)

const (
	// UserRoleUser 表示普通用户
	UserRoleUser uint8 = 1
	// UserRoleAdmin 表示平台管理员
	UserRoleAdmin uint8 = 2

	// UserStatusActive 表示账号可以登录和管理资源
	UserStatusActive uint8 = 1
	// UserStatusDisabled 表示账号已被管理员停用
	UserStatusDisabled uint8 = 2

	// MemberRoleOwner 表示工作空间所有者
	MemberRoleOwner uint8 = 1
	// MemberRoleAdmin 表示后续团队功能中的工作空间管理员
	MemberRoleAdmin uint8 = 2
	// MemberRoleMember 表示后续团队功能中的普通成员
	MemberRoleMember uint8 = 3
)

// User 是可登录账号的业务模型
type User struct {
	// ID 是 ULID 用户标识
	ID string `db:"id"`
	// Email 是规范化后的唯一登录邮箱
	Email string `db:"email"`
	// PasswordHash 是 Argon2id 摘要，禁止返回客户端
	PasswordHash string `db:"password_hash"`
	// DisplayName 是工作空间和界面展示名称
	DisplayName string `db:"display_name"`
	// Role 是平台角色，取值为 UserRoleUser 或 UserRoleAdmin
	Role uint8 `db:"role"`
	// Status 是账号状态，取值为 UserStatusActive 或 UserStatusDisabled
	Status uint8 `db:"status"`
	// CreatedAt 是账号创建的 UTC 时间
	CreatedAt time.Time `db:"created_at"`
	// UpdatedAt 是账号最近修改的 UTC 时间
	UpdatedAt time.Time `db:"updated_at"`
}

// Workspace 是链接、域名、二维码和分析数据的归属空间
type Workspace struct {
	// ID 是 ULID 工作空间标识
	ID string `db:"id"`
	// Name 是工作空间展示名称
	Name string `db:"name"`
	// Slug 是工作空间唯一短标识
	Slug string `db:"slug"`
	// OwnerUserID 是创建者的 ULID 用户标识
	OwnerUserID string `db:"owner_user_id"`
	// CreatedAt 是工作空间创建的 UTC 时间
	CreatedAt time.Time `db:"created_at"`
	// UpdatedAt 是工作空间最近修改的 UTC 时间
	UpdatedAt time.Time `db:"updated_at"`
}

// RefreshToken 保存不透明 Refresh Token 的摘要与轮换状态
type RefreshToken struct {
	// ID 是 ULID Token 记录标识
	ID string `db:"id"`
	// UserID 是 Token 所属用户的 ULID 标识
	UserID string `db:"user_id"`
	// TokenHash 是原始 Token 的 SHA-256 摘要
	TokenHash []byte `db:"token_hash"`
	// FamilyID 是同一登录轮换链的 ULID 族标识
	FamilyID string `db:"family_id"`
	// ExpiresAt 是 Token 失效的 UTC 时间
	ExpiresAt time.Time `db:"expires_at"`
	// RevokedAt 是 Token 被撤销的 UTC 时间，nil 表示未撤销
	RevokedAt *time.Time `db:"revoked_at"`
	// ReplacedBy 是轮换后新 Token 记录的 ULID 标识
	ReplacedBy *string `db:"replaced_by"`
	// CreatedAt 是 Token 签发的 UTC 时间
	CreatedAt time.Time `db:"created_at"`
}

// AuthResult 是注册、登录或 Token 轮换后的安全业务结果
type AuthResult struct {
	// User 是脱敏后的用户模型
	User User
	// Workspace 是当前默认工作空间
	Workspace Workspace
	// AccessToken 是访问受保护接口的 JWT
	AccessToken string
	// AccessTokenExpires 是 Access Token 的 UTC 过期时间
	AccessTokenExpires time.Time
	// RefreshToken 是只返回一次给客户端的原始轮换凭证
	RefreshToken string
	// RefreshTokenExpires 是 Refresh Token 的 UTC 过期时间
	RefreshTokenExpires time.Time
}
