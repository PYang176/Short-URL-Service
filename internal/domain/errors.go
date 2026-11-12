package domain

import "errors"

var (
	// ErrInvalidDomain 表示域名格式或验证参数无效
	ErrInvalidDomain = errors.New("域名参数无效")
	// ErrDomainExists 表示域名已被其他工作空间占用
	ErrDomainExists = errors.New("域名已存在")
	// ErrNotFound 表示域名不存在
	ErrNotFound = errors.New("域名不存在")
	// ErrNotVerified 表示域名尚未完成验证
	ErrNotVerified = errors.New("域名尚未验证")
	// ErrVerificationFailed 表示 DNS TXT 或 CNAME 验证失败
	ErrVerificationFailed = errors.New("域名验证失败")
	// ErrDomainInUse 表示域名仍有链接，不能删除
	ErrDomainInUse = errors.New("域名仍在使用")
)
