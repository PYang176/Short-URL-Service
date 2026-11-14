package analytics

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

// Summary 是链接在时间范围内的核心指标
type Summary struct {
	// TotalClicks 是全部点击
	TotalClicks int64 `json:"total_clicks" db:"total_clicks"`
	// HumanClicks 是未命中手写 UA 机器人规则的点击
	HumanClicks int64 `json:"human_clicks" db:"human_clicks"`
	// BotClicks 是命中手写 UA 机器人规则的点击
	BotClicks int64 `json:"bot_clicks" db:"bot_clicks"`
	// UniqueVisitors 是按访客哈希去重后的数量
	UniqueVisitors int64 `json:"unique_visitors" db:"unique_visitors"`
}

// TrendPoint 是一个时间桶内的点击数量
type TrendPoint struct {
	// Bucket 是 SQL 格式化后的时间桶起点
	Bucket string `json:"bucket" db:"bucket"`
	// HumanClicks 是人类点击
	HumanClicks int64 `json:"human_clicks" db:"human_clicks"`
	// BotClicks 是机器人点击
	BotClicks int64 `json:"bot_clicks" db:"bot_clicks"`
}

// BreakdownItem 是单个维度值的点击数量
type BreakdownItem struct {
	// Value 是维度值
	Value string `json:"value" db:"value"`
	// Clicks 是人类点击数量
	Clicks int64 `json:"clicks" db:"clicks"`
}

// Overview 是链接分析接口的完整结果
type Overview struct {
	// Days 是查询覆盖的天数
	Days int `json:"days"`
	// Summary 是核心指标
	Summary Summary `json:"summary"`
	// Trend 是按小时或天的点击趋势
	Trend []TrendPoint `json:"trend"`
	// Countries 是国家分布
	Countries []BreakdownItem `json:"countries"`
	// Devices 是设备分布
	Devices []BreakdownItem `json:"devices"`
	// OperatingSystems 是操作系统分布
	OperatingSystems []BreakdownItem `json:"operating_systems"`
	// Browsers 是浏览器分布
	Browsers []BreakdownItem `json:"browsers"`
	// Referrers 是来源页分布
	Referrers []BreakdownItem `json:"referrers"`
}

// WorkspaceSummary 是工作空间整体指标
type WorkspaceSummary struct {
	// TotalLinks 是未删除链接总数
	TotalLinks int64 `json:"total_links" db:"total_links"`
	// ActiveLinks 是当前处于启用和有效时间窗口内的链接数
	ActiveLinks int64 `json:"active_links" db:"active_links"`
	// TotalClicks 是时间范围内全部点击
	TotalClicks int64 `json:"total_clicks" db:"total_clicks"`
	// HumanClicks 是时间范围内未命中 UA 规则的点击
	HumanClicks int64 `json:"human_clicks" db:"human_clicks"`
	// BotClicks 是时间范围内疑似机器人点击
	BotClicks int64 `json:"bot_clicks" db:"bot_clicks"`
	// UniqueVisitors 是时间范围内按访客哈希去重的数量
	UniqueVisitors int64 `json:"unique_visitors" db:"unique_visitors"`
}

// TopLink 是工作空间点击量最高的链接
type TopLink struct {
	// ID 是 ULID 链接标识
	ID string `json:"id" db:"id"`
	// Slug 是短链短码
	Slug string `json:"slug" db:"slug"`
	// Title 是链接标题
	Title string `json:"title" db:"title"`
	// Status 是链接状态
	Status uint8 `json:"status" db:"status"`
	// Clicks 是时间范围内点击数量
	Clicks int64 `json:"clicks" db:"clicks"`
}

// WorkspaceOverview 是工作空间整体分析结果
type WorkspaceOverview struct {
	// Days 是查询覆盖的天数
	Days int `json:"days"`
	// Summary 是工作空间核心指标
	Summary WorkspaceSummary `json:"summary"`
	// Trend 是工作空间整体点击趋势
	Trend []TrendPoint `json:"trend"`
	// TopLinks 是点击量最高的链接
	TopLinks []TopLink `json:"top_links"`
}

// ExportFilter 是点击明细导出使用的组合筛选条件
type ExportFilter struct {
	// WorkspaceID 是必须匹配的工作空间 ULID
	WorkspaceID string
	// LinkID 是可选短链 ULID；空值表示整个工作空间
	LinkID string
	// From 是点击时间的包含下界 UTC 时间
	From time.Time
	// To 是点击时间的排除上界 UTC 时间
	To time.Time
	// IsBot 为 nil 时不过滤，false 只导出真人，true 只导出机器人
	IsBot *bool
	// Device 是可选设备精确匹配
	Device string
	// Country 是可选 ISO 国家代码精确匹配
	Country string
	// Browser 是可选浏览器精确匹配
	Browser string
	// Referrer 是可选来源页精确匹配
	Referrer string
	// UTMSource 是可选 utm_source 精确匹配
	UTMSource string
}

// ExportEvent 是一条可安全导出的点击明细
type ExportEvent struct {
	// EventID 是 ULID 点击事件标识
	EventID string `db:"event_id"`
	// ClickedAt 是点击 UTC 时间
	ClickedAt time.Time `db:"clicked_at"`
	// Slug 是点击时的短码快照
	Slug string `db:"slug"`
	// Country 是 ISO 国家代码
	Country string `db:"country"`
	// Device 是设备类型
	Device string `db:"device"`
	// OS 是操作系统
	OS string `db:"os"`
	// Browser 是浏览器
	Browser string `db:"browser"`
	// Language 是浏览器语言
	Language string `db:"language"`
	// Referrer 是来源页
	Referrer string `db:"referrer"`
	// UTMSource 是 utm_source
	UTMSource string `db:"utm_source"`
	// UTMMedium 是 utm_medium
	UTMMedium string `db:"utm_medium"`
	// UTMCampaign 是 utm_campaign
	UTMCampaign string `db:"utm_campaign"`
	// UTMTerm 是 utm_term
	UTMTerm string `db:"utm_term"`
	// UTMContent 是 utm_content
	UTMContent string `db:"utm_content"`
	// IsBot 表示是否命中项目内机器人 UA 规则
	IsBot bool `db:"is_bot"`
}

// ExportReader 是点击明细的流式读取器
type ExportReader interface {
	// Next 前进到下一条记录，False 表示结束或读取失败
	Next() bool
	// Event 返回当前记录
	Event() ExportEvent
	// Err 返回迭代过程中的错误
	Err() error
	// Close 释放数据库游标
	Close() error
}

// Repository 定义点击事件存储和分析查询
type Repository interface {
	// InsertBatch 幂等批量保存点击事件
	InsertBatch(ctx context.Context, events []Event) (int64, error)
	// Summary 查询时间范围内的核心指标
	Summary(ctx context.Context, workspaceID, linkID string, from, to time.Time) (Summary, error)
	// Trend 查询按小时或天分组的趋势
	Trend(ctx context.Context, workspaceID, linkID string, from, to time.Time, daily bool) ([]TrendPoint, error)
	// Breakdown 查询指定安全维度的前 10 项
	Breakdown(ctx context.Context, workspaceID, linkID string, from, to time.Time, field string) ([]BreakdownItem, error)
	// WorkspaceSummary 查询工作空间整体指标
	WorkspaceSummary(
		ctx context.Context,
		workspaceID string,
		now, from, to time.Time,
	) (WorkspaceSummary, error)
	// WorkspaceTrend 查询工作空间整体点击趋势
	WorkspaceTrend(
		ctx context.Context,
		workspaceID string,
		from, to time.Time,
		daily bool,
	) ([]TrendPoint, error)
	// TopLinks 查询工作空间点击量最高的链接
	TopLinks(
		ctx context.Context,
		workspaceID string,
		from, to time.Time,
		limit int,
	) ([]TopLink, error)
	// OpenExport 打开带组合筛选的点击明细游标
	OpenExport(ctx context.Context, filter ExportFilter) (ExportReader, error)
}

// MySQLRepository 使用 MySQL 保存和查询点击事件
type MySQLRepository struct {
	db *sqlx.DB
}

// NewMySQLRepository 创建点击分析仓库
func NewMySQLRepository(db *sqlx.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

// InsertBatch 使用单条多行 INSERT IGNORE 幂等保存点击事件
func (r *MySQLRepository) InsertBatch(ctx context.Context, events []Event) (int64, error) {
	if len(events) == 0 {
		return 0, nil
	}

	linkWorkspaces, err := r.loadLinkWorkspaces(ctx, events)
	if err != nil {
		return 0, err
	}

	const columnsPerEvent = 19
	values := make([]string, 0, len(events))
	args := make([]any, 0, len(events)*columnsPerEvent)
	for _, event := range events {
		if workspaceID, ok := linkWorkspaces[event.LinkID]; ok {
			event.WorkspaceID = workspaceID
		}
		values = append(values, "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
		args = append(args,
			event.EventID, event.WorkspaceID, event.LinkID, event.DomainID, event.Slug,
			event.ClickedAt, event.VisitorHash, event.Country, event.Device, event.OS,
			event.Browser, event.Language, event.Referrer, event.UTMSource, event.UTMMedium,
			event.UTMCampaign, event.UTMTerm, event.UTMContent, event.IsBot,
		)
	}

	query := `
		INSERT IGNORE INTO click_events (
			event_id, workspace_id, link_id, domain_id, slug, clicked_at,
			visitor_hash, country, device, os, browser, language, referrer,
			utm_source, utm_medium, utm_campaign, utm_term, utm_content, is_bot
		) VALUES ` + strings.Join(values, ",")
	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("批量写入点击事件失败: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("读取点击事件写入行数失败: %w", err)
	}
	return inserted, nil
}

// loadLinkWorkspaces 读取链接当前归属，修正认领前已入队点击的 workspace
func (r *MySQLRepository) loadLinkWorkspaces(
	ctx context.Context,
	events []Event,
) (map[string]string, error) {
	linkIDs := make([]string, 0, len(events))
	seen := make(map[string]struct{}, len(events))
	for _, event := range events {
		if _, ok := seen[event.LinkID]; ok {
			continue
		}
		seen[event.LinkID] = struct{}{}
		linkIDs = append(linkIDs, event.LinkID)
	}

	query, args, err := sqlx.In(`
		SELECT id, workspace_id
		FROM links
		WHERE id IN (?)`, linkIDs)
	if err != nil {
		return nil, fmt.Errorf("构造点击链接归属查询失败: %w", err)
	}

	type linkWorkspace struct {
		ID          string `db:"id"`
		WorkspaceID string `db:"workspace_id"`
	}
	rows := make([]linkWorkspace, 0, len(linkIDs))
	if err := r.db.SelectContext(
		ctx,
		&rows,
		r.db.Rebind(query),
		args...,
	); err != nil {
		return nil, fmt.Errorf("查询点击链接归属失败: %w", err)
	}

	result := make(map[string]string, len(rows))
	for _, row := range rows {
		result[row.ID] = row.WorkspaceID
	}
	return result, nil
}

// Summary 查询时间范围内的核心指标
func (r *MySQLRepository) Summary(
	ctx context.Context,
	workspaceID, linkID string,
	from, to time.Time,
) (Summary, error) {
	var summary Summary
	err := r.db.GetContext(ctx, &summary, `
		SELECT
			COUNT(*) AS total_clicks,
			COALESCE(SUM(is_bot = 0), 0) AS human_clicks,
			COALESCE(SUM(is_bot = 1), 0) AS bot_clicks,
			COUNT(DISTINCT CASE WHEN is_bot = 0 THEN visitor_hash END) AS unique_visitors
		FROM click_events
		WHERE workspace_id = ?
		  AND link_id = ?
		  AND clicked_at >= ?
		  AND clicked_at < ?`,
		workspaceID, linkID, from, to,
	)
	if err != nil {
		return Summary{}, fmt.Errorf("查询点击汇总失败: %w", err)
	}
	return summary, nil
}

// Trend 查询按小时或天分组的趋势
func (r *MySQLRepository) Trend(
	ctx context.Context,
	workspaceID, linkID string,
	from, to time.Time,
	daily bool,
) ([]TrendPoint, error) {
	format := "%Y-%m-%d %H:00:00"
	if daily {
		format = "%Y-%m-%d 00:00:00"
	}

	query := `
		SELECT
			DATE_FORMAT(clicked_at, ?) AS bucket,
			COALESCE(SUM(is_bot = 0), 0) AS human_clicks,
			COALESCE(SUM(is_bot = 1), 0) AS bot_clicks
		FROM click_events
		WHERE workspace_id = ?
		  AND link_id = ?
		  AND clicked_at >= ?
		  AND clicked_at < ?
		GROUP BY bucket
		ORDER BY bucket ASC`
	points := make([]TrendPoint, 0)
	if err := r.db.SelectContext(ctx, &points, query, format, workspaceID, linkID, from, to); err != nil {
		return nil, fmt.Errorf("查询点击趋势失败: %w", err)
	}
	return points, nil
}

// Breakdown 查询指定安全维度的前 10 项
func (r *MySQLRepository) Breakdown(
	ctx context.Context,
	workspaceID, linkID string,
	from, to time.Time,
	field string,
) ([]BreakdownItem, error) {
	column, ok := breakdownColumns[field]
	if !ok {
		return nil, fmt.Errorf("不支持的点击分析维度 %q", field)
	}

	query := fmt.Sprintf(`
		SELECT
			COALESCE(NULLIF(CONVERT(%s USING utf8mb4), ''), 'unknown') AS value,
			COUNT(*) AS clicks
		FROM click_events
		WHERE workspace_id = ?
		  AND link_id = ?
		  AND clicked_at >= ?
		  AND clicked_at < ?
		  AND is_bot = 0
		GROUP BY value
		ORDER BY clicks DESC, value ASC
		LIMIT 10`, column)
	items := make([]BreakdownItem, 0, 10)
	if err := r.db.SelectContext(ctx, &items, query, workspaceID, linkID, from, to); err != nil {
		return nil, fmt.Errorf("查询点击维度失败: %w", err)
	}
	return items, nil
}

// WorkspaceSummary 查询工作空间链接数和整体点击指标
func (r *MySQLRepository) WorkspaceSummary(
	ctx context.Context,
	workspaceID string,
	now, from, to time.Time,
) (WorkspaceSummary, error) {
	var summary WorkspaceSummary
	if err := r.db.GetContext(ctx, &summary, `
		SELECT
			COUNT(*) AS total_links,
			COALESCE(SUM(
				status = 1
				AND (starts_at IS NULL OR starts_at <= ?)
				AND (expires_at IS NULL OR expires_at > ?)
			), 0) AS active_links
		FROM links
		WHERE workspace_id = ? AND deleted_at IS NULL`,
		now, now, workspaceID,
	); err != nil {
		return WorkspaceSummary{}, fmt.Errorf("查询工作空间链接汇总失败: %w", err)
	}

	var clicks Summary
	if err := r.db.GetContext(ctx, &clicks, `
		SELECT
			COUNT(*) AS total_clicks,
			COALESCE(SUM(is_bot = 0), 0) AS human_clicks,
			COALESCE(SUM(is_bot = 1), 0) AS bot_clicks,
			COUNT(DISTINCT CASE WHEN is_bot = 0 THEN visitor_hash END) AS unique_visitors
		FROM click_events
		WHERE workspace_id = ?
		  AND clicked_at >= ?
		  AND clicked_at < ?`,
		workspaceID, from, to,
	); err != nil {
		return WorkspaceSummary{}, fmt.Errorf("查询工作空间点击汇总失败: %w", err)
	}
	summary.TotalClicks = clicks.TotalClicks
	summary.HumanClicks = clicks.HumanClicks
	summary.BotClicks = clicks.BotClicks
	summary.UniqueVisitors = clicks.UniqueVisitors
	return summary, nil
}

// WorkspaceTrend 查询工作空间整体点击趋势
func (r *MySQLRepository) WorkspaceTrend(
	ctx context.Context,
	workspaceID string,
	from, to time.Time,
	daily bool,
) ([]TrendPoint, error) {
	format := "%Y-%m-%d %H:00:00"
	if daily {
		format = "%Y-%m-%d 00:00:00"
	}
	points := make([]TrendPoint, 0)
	if err := r.db.SelectContext(ctx, &points, `
		SELECT
			DATE_FORMAT(clicked_at, ?) AS bucket,
			COALESCE(SUM(is_bot = 0), 0) AS human_clicks,
			COALESCE(SUM(is_bot = 1), 0) AS bot_clicks
		FROM click_events
		WHERE workspace_id = ?
		  AND clicked_at >= ?
		  AND clicked_at < ?
		GROUP BY bucket
		ORDER BY bucket ASC`,
		format, workspaceID, from, to,
	); err != nil {
		return nil, fmt.Errorf("查询工作空间点击趋势失败: %w", err)
	}
	return points, nil
}

// TopLinks 查询工作空间点击量最高的链接
func (r *MySQLRepository) TopLinks(
	ctx context.Context,
	workspaceID string,
	from, to time.Time,
	limit int,
) ([]TopLink, error) {
	items := make([]TopLink, 0, limit)
	if err := r.db.SelectContext(ctx, &items, `
		SELECT
			l.id,
			l.slug,
			l.title,
			l.status,
			COUNT(ce.event_id) AS clicks
		FROM links AS l
		LEFT JOIN click_events AS ce
			ON ce.workspace_id = l.workspace_id
			AND ce.link_id = l.id
			AND ce.clicked_at >= ?
			AND ce.clicked_at < ?
		WHERE l.workspace_id = ? AND l.deleted_at IS NULL
		GROUP BY l.id, l.slug, l.title, l.status, l.created_at
		ORDER BY clicks DESC, l.created_at DESC
		LIMIT ?`,
		from, to, workspaceID, limit,
	); err != nil {
		return nil, fmt.Errorf("查询工作空间热门链接失败: %w", err)
	}
	return items, nil
}

// OpenExport 打开带组合筛选的点击明细游标
func (r *MySQLRepository) OpenExport(
	ctx context.Context,
	filter ExportFilter,
) (ExportReader, error) {
	query := `
		SELECT event_id, clicked_at, slug, country, device, os, browser,
			language, referrer, utm_source, utm_medium, utm_campaign,
			utm_term, utm_content, is_bot
		FROM click_events
		WHERE workspace_id = ?
		  AND clicked_at >= ?
		  AND clicked_at < ?`
	args := []any{filter.WorkspaceID, filter.From, filter.To}
	if filter.LinkID != "" {
		query += " AND link_id = ?"
		args = append(args, filter.LinkID)
	}
	if filter.IsBot != nil {
		query += " AND is_bot = ?"
		args = append(args, *filter.IsBot)
	}
	filters := []struct {
		column string
		value  string
	}{
		{column: "device", value: filter.Device},
		{column: "country", value: filter.Country},
		{column: "browser", value: filter.Browser},
		{column: "referrer", value: filter.Referrer},
		{column: "utm_source", value: filter.UTMSource},
	}
	for _, item := range filters {
		if item.value != "" {
			query += " AND " + item.column + " = ?"
			args = append(args, item.value)
		}
	}
	query += " ORDER BY clicked_at DESC, event_id DESC"

	rows, err := r.db.QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("打开点击明细导出失败: %w", err)
	}
	return &sqlExportReader{rows: rows}, nil
}

type sqlExportReader struct {
	rows    *sqlx.Rows
	current ExportEvent
	err     error
}

// Next 读取下一条点击明细
func (r *sqlExportReader) Next() bool {
	if !r.rows.Next() {
		return false
	}
	if err := r.rows.StructScan(&r.current); err != nil {
		r.err = fmt.Errorf("读取点击明细失败: %w", err)
		return false
	}
	return true
}

// Event 返回当前点击明细
func (r *sqlExportReader) Event() ExportEvent {
	return r.current
}

// Err 返回游标读取错误
func (r *sqlExportReader) Err() error {
	if r.err != nil {
		return r.err
	}
	return r.rows.Err()
}

// Close 关闭数据库游标
func (r *sqlExportReader) Close() error {
	if err := r.rows.Close(); err != nil {
		return fmt.Errorf("关闭点击明细游标失败: %w", err)
	}
	return nil
}

var breakdownColumns = map[string]string{
	"country":    "country",
	"device":     "device",
	"os":         "os",
	"browser":    "browser",
	"referrer":   "referrer",
	"utm_source": "utm_source",
}
