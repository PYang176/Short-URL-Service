package model

// URL 核心数据结构
type URL struct {
	ID          int64  `json:"id" db:"id"`
	OriginalURL string `json:"original_url" db:"original_url"` // 原始URL
	ShortCode   string `json:"short_code" db:"short_code"`     // 短链码 (e.g., "abc123")
	IsCustom    bool   `json:"is_custom" db:"is_custom"`       // 是否自定义别名
	Clicks      int64  `json:"clicks" db:"clicks"`             // 点击量
	CreatedAt   int64  `json:"created_at" db:"created_at"`     // 创建时间
	ExpiredAt   int64  `json:"expired_at" db:"expired_at"`     // 过期时间
}

// CreateRequest 创建短链的请求体
type CreateRequest struct {
	OriginalURL  string `json:"original_url" binding:"required"`
	CustomCode   string `json:"custom_code,omitempty"`   // 用户传来的自定义别名
	ExpiredAt    int64  `json:"expired_at,omitempty"`    // 过期时间戳
	ForceRefresh bool   `json:"force_refresh,omitempty"` // 是否强制生成新链
}
