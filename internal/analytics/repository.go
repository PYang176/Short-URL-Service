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
	// HumanClicks 是机器人过滤后的点击
	HumanClicks int64 `json:"human_clicks" db:"human_clicks"`
	// BotClicks 是识别出的机器人点击
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

	const columnsPerEvent = 19
	values := make([]string, 0, len(events))
	args := make([]any, 0, len(events)*columnsPerEvent)
	for _, event := range events {
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

var breakdownColumns = map[string]string{
	"country":    "country",
	"device":     "device",
	"os":         "os",
	"browser":    "browser",
	"referrer":   "referrer",
	"utm_source": "utm_source",
}
