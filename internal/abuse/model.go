package abuse

import "time"

const (
	// ReportStatusPending 表示举报等待管理员处理
	ReportStatusPending uint8 = 1
	// ReportStatusTakenDown 表示举报成立且短链已下架
	ReportStatusTakenDown uint8 = 2
	// ReportStatusRejected 表示举报不成立
	ReportStatusRejected uint8 = 3

	// ReasonPhishing 表示钓鱼欺诈
	ReasonPhishing = "phishing"
	// ReasonMalware 表示恶意软件
	ReasonMalware = "malware"
	// ReasonSpam 表示垃圾推广
	ReasonSpam = "spam"
	// ReasonImpersonation 表示仿冒身份
	ReasonImpersonation = "impersonation"
	// ReasonOther 表示其他违规
	ReasonOther = "other"

	// DecisionTakedown 表示确认违规并下架短链
	DecisionTakedown = "takedown"
	// DecisionReject 表示驳回举报
	DecisionReject = "reject"
)

// BlockedDomain 是禁止创建或修改到的目标域名规则
type BlockedDomain struct {
	// ID 是 ULID 黑名单记录标识
	ID string `db:"id" json:"id"`
	// Hostname 是小写域名或 IP，规则同时匹配其子域名
	Hostname string `db:"hostname" json:"hostname"`
	// Reason 是管理员填写的阻断原因
	Reason string `db:"reason" json:"reason"`
	// CreatedByUserID 是添加规则的 ULID 管理员标识
	CreatedByUserID string `db:"created_by_user_id" json:"-"`
	// DeletedAt 是规则停用的 UTC 时间，nil 表示有效
	DeletedAt *time.Time `db:"deleted_at" json:"deleted_at"`
	// CreatedAt 是规则创建的 UTC 时间
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	// UpdatedAt 是规则最近修改的 UTC 时间
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// Report 是访客提交的短链滥用举报
type Report struct {
	// ID 是 ULID 举报标识
	ID string `db:"id" json:"id"`
	// LinkID 是被举报的 ULID 短链标识
	LinkID string `db:"link_id" json:"link_id"`
	// WorkspaceID 是短链所属的 ULID 工作空间标识
	WorkspaceID string `db:"workspace_id" json:"-"`
	// ReporterUserID 是可选举报用户 ULID；匿名举报为 nil
	ReporterUserID *string `db:"reporter_user_id" json:"-"`
	// Reason 是 phishing、malware、spam、impersonation 或 other
	Reason string `db:"reason" json:"reason"`
	// Details 是用户补充说明
	Details string `db:"details" json:"details"`
	// Status 是 ReportStatusPending、ReportStatusTakenDown 或 ReportStatusRejected
	Status uint8 `db:"status" json:"status"`
	// ResolutionNote 是管理员处理说明
	ResolutionNote string `db:"resolution_note" json:"resolution_note"`
	// ResolvedByUserID 是处理管理员 ULID
	ResolvedByUserID *string `db:"resolved_by_user_id" json:"-"`
	// ResolvedAt 是处理完成的 UTC 时间
	ResolvedAt *time.Time `db:"resolved_at" json:"resolved_at"`
	// CreatedAt 是举报提交的 UTC 时间
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	// UpdatedAt 是举报最近修改的 UTC 时间
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
	// LinkTitle 是被举报短链的标题
	LinkTitle string `db:"link_title" json:"link_title"`
	// LinkSlug 是被举报短链的短码
	LinkSlug string `db:"link_slug" json:"link_slug"`
	// DomainHostname 是短链所在域名
	DomainHostname string `db:"domain_hostname" json:"domain_hostname"`
	// DestinationURL 是被举报链接的目标地址
	DestinationURL string `db:"destination_url" json:"destination_url"`
	// LinkStatus 是被举报短链当前状态
	LinkStatus uint8 `db:"link_status" json:"link_status"`
}

// ReportResolution 是一次举报处理产生的缓存失效信息
type ReportResolution struct {
	// Report 是处理后的举报记录
	Report Report
	// DomainID 是被下架短链的域名标识；驳回时为空
	DomainID string
	// Slug 是被下架短链的短码；驳回时为空
	Slug string
}

// CreateReportResult 是公开举报接口返回的最小结果
type CreateReportResult struct {
	// ID 是 ULID 举报标识
	ID string
	// Status 是举报初始状态
	Status uint8
}
