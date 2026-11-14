package trial

import "errors"

var (
	// ErrSessionNotFound 表示 Cookie 没有对应的游客会话
	ErrSessionNotFound = errors.New("游客会话不存在")
	// ErrSessionExpired 表示游客会话已经过期
	ErrSessionExpired = errors.New("游客会话已过期")
	// ErrLinkLimitReached 表示游客工作空间已达到链接数量上限
	ErrLinkLimitReached = errors.New("游客链接数量已达上限")
	// ErrInvalidExpiry 表示试用链接有效期不在允许选项中
	ErrInvalidExpiry = errors.New("试用链接有效期无效")
	// ErrClaimConflict 表示目标工作空间已存在相同域名短码
	ErrClaimConflict = errors.New("游客链接与目标工作空间冲突")
	// ErrClaimTargetNotFound 表示认领目标不是可用的正式工作空间
	ErrClaimTargetNotFound = errors.New("认领目标工作空间不存在")
)
