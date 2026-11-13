package webhook

import "errors"

var (
	// ErrInvalidInput 表示名称、URL 或投递参数无效
	ErrInvalidInput = errors.New("webhook 参数无效")
	// ErrNotFound 表示当前工作空间的 Webhook 不存在
	ErrNotFound = errors.New("webhook 不存在")
	// ErrUnsafeTarget 表示目标地址违反 SSRF 或传输安全策略
	ErrUnsafeTarget = errors.New("webhook 目标地址不安全")
	// ErrLeaseLost 表示 Worker 已失去投递任务的租约
	ErrLeaseLost = errors.New("webhook 投递租约已失效")
)
