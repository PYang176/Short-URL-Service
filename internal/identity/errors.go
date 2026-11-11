package identity

import "errors"

var (
	// ErrInvalidInput 表示身份请求参数不满足业务约束
	ErrInvalidInput = errors.New("身份请求参数无效")
	// ErrEmailExists 表示邮箱已经被注册
	ErrEmailExists = errors.New("邮箱已注册")
	// ErrInvalidCredentials 表示邮箱或密码错误
	ErrInvalidCredentials = errors.New("邮箱或密码错误")
	// ErrUnauthorized 表示认证凭证缺失、无效或已过期
	ErrUnauthorized = errors.New("身份认证失败")
	// ErrTokenReuse 表示已使用过的 Refresh Token 再次出现
	ErrTokenReuse = errors.New("检测到 Refresh Token 重放")
	// ErrNotFound 表示身份或工作空间记录不存在
	ErrNotFound = errors.New("身份记录不存在")
)
