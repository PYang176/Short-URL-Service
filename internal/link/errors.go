package link

import "errors"

var (
	// ErrInvalidInput 表示目标地址、短码或时间范围不合法
	ErrInvalidInput = errors.New("短链参数无效")
	// ErrNotFound 表示链接或域名不存在
	ErrNotFound = errors.New("短链不存在")
	// ErrGone 表示链接已停用、删除或过期
	ErrGone = errors.New("短链已失效")
	// ErrNotStarted 表示链接尚未到生效时间
	ErrNotStarted = errors.New("短链尚未生效")
	// ErrSlugTaken 表示自定义短码已被占用
	ErrSlugTaken = errors.New("短码已被占用")
)
