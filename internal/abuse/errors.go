package abuse

import "errors"

var (
	// ErrInvalidInput 表示举报、短链地址或域名参数无效
	ErrInvalidInput = errors.New("abuse 参数无效")
	// ErrNotFound 表示举报或黑名单记录不存在
	ErrNotFound = errors.New("abuse 记录不存在")
	// ErrAlreadyResolved 表示举报已经处理完成
	ErrAlreadyResolved = errors.New("abuse 举报已处理")
)
