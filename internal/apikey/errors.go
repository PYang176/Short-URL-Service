package apikey

import "errors"

var (
	// ErrInvalidInput 表示名称、作用域或过期时间无效
	ErrInvalidInput = errors.New("API Key 参数无效")
	// ErrNotFound 表示当前工作空间不存在该 API Key
	ErrNotFound = errors.New("API Key 不存在")
	// ErrUnauthorized 表示 API Key 格式、状态或密钥校验失败
	ErrUnauthorized = errors.New("API Key 认证失败")
	// ErrPrefixConflict 表示随机前缀发生唯一键冲突
	ErrPrefixConflict = errors.New("API Key 前缀冲突")
)
