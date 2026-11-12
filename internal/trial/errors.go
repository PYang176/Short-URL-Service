package trial

import "errors"

var (
	// ErrSessionNotFound 表示 Cookie 没有对应的游客会话
	ErrSessionNotFound = errors.New("游客会话不存在")
	// ErrSessionExpired 表示游客会话已经过期
	ErrSessionExpired = errors.New("游客会话已过期")
	// ErrLinkLimitReached 表示游客工作空间已达到链接数量上限
	ErrLinkLimitReached = errors.New("游客链接数量已达上限")
)
