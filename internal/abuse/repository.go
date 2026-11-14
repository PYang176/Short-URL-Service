package abuse

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/oklog/ulid/v2"
)

// Repository 定义滥用举报、黑名单和审计日志的持久化能力
type Repository interface {
	// CreateReport 保存新的待处理举报
	CreateReport(ctx context.Context, report *Report) error
	// ListReports 按状态返回最近举报及关联链接信息
	ListReports(ctx context.Context, status uint8, limit int) ([]Report, error)
	// ResolveReport 原子处理举报；确认违规时在同一事务中停用短链
	ResolveReport(
		ctx context.Context,
		reportID, adminUserID, decision, note string,
		now time.Time,
	) (*ReportResolution, error)
	// ListBlockedDomains 返回当前有效的目标域名黑名单
	ListBlockedDomains(ctx context.Context) ([]BlockedDomain, error)
	// AddBlockedDomain 新增或恢复域名规则并写入审计日志
	AddBlockedDomain(ctx context.Context, domain *BlockedDomain) error
	// RemoveBlockedDomain 停用域名规则并写入审计日志
	RemoveBlockedDomain(ctx context.Context, id, adminUserID string, now time.Time) error
	// IsDestinationBlocked 判断目标 Host 是否命中有效黑名单规则
	IsDestinationBlocked(ctx context.Context, hostname string) (bool, error)
}

// MySQLRepository 使用 MySQL 保存治理数据
type MySQLRepository struct {
	db *sqlx.DB
}

// NewMySQLRepository 创建滥用治理仓库
func NewMySQLRepository(db *sqlx.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

// CreateReport 保存新的待处理举报
func (r *MySQLRepository) CreateReport(ctx context.Context, report *Report) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO abuse_reports (
			id, link_id, workspace_id, reporter_user_id, reason, details,
			status, resolution_note, resolved_by_user_id, resolved_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		report.ID, report.LinkID, report.WorkspaceID, report.ReporterUserID,
		report.Reason, report.Details, report.Status, report.ResolutionNote,
		report.ResolvedByUserID, report.ResolvedAt, report.CreatedAt, report.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("保存滥用举报失败: %w", err)
	}
	return nil
}

// ListReports 按状态返回最近举报及关联链接信息
func (r *MySQLRepository) ListReports(
	ctx context.Context,
	status uint8,
	limit int,
) ([]Report, error) {
	reports := make([]Report, 0, limit)
	if err := r.db.SelectContext(ctx, &reports, `
		SELECT r.id, r.link_id, r.workspace_id, r.reporter_user_id, r.reason,
			r.details, r.status, r.resolution_note, r.resolved_by_user_id,
			r.resolved_at, r.created_at, r.updated_at,
			l.title AS link_title, l.slug AS link_slug, l.destination_url,
			l.status AS link_status, d.hostname AS domain_hostname
		FROM abuse_reports AS r
		INNER JOIN links AS l ON l.id = r.link_id
		INNER JOIN domains AS d ON d.id = l.domain_id
		WHERE r.status = ?
		ORDER BY r.created_at DESC, r.id DESC
		LIMIT ?`, status, limit); err != nil {
		return nil, fmt.Errorf("查询滥用举报失败: %w", err)
	}
	return reports, nil
}

// ResolveReport 原子处理举报；确认违规时在同一事务中停用短链
func (r *MySQLRepository) ResolveReport(
	ctx context.Context,
	reportID, adminUserID, decision, note string,
	now time.Time,
) (*ReportResolution, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("开始举报处理事务失败: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var current Report
	if err := tx.GetContext(ctx, &current, `
		SELECT id, link_id, workspace_id, status
		FROM abuse_reports
		WHERE id = ?
		FOR UPDATE`, reportID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("锁定滥用举报失败: %w", err)
	}
	if current.Status != ReportStatusPending {
		return nil, ErrAlreadyResolved
	}

	resolution := &ReportResolution{Report: current}
	reportStatus := ReportStatusRejected
	action := "abuse.report.reject"
	if decision == DecisionTakedown {
		var linkTarget struct {
			DomainID string `db:"domain_id"`
			Slug     string `db:"slug"`
		}
		if err := tx.GetContext(ctx, &linkTarget, `
			SELECT domain_id, slug
			FROM links
			WHERE id = ? AND deleted_at IS NULL
			FOR UPDATE`, current.LinkID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrNotFound
			}
			return nil, fmt.Errorf("锁定被举报短链失败: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE links
			SET status = 2, version = version + 1, updated_at = ?
			WHERE id = ? AND deleted_at IS NULL`,
			now, current.LinkID,
		); err != nil {
			return nil, fmt.Errorf("下架被举报短链失败: %w", err)
		}
		resolution.DomainID = linkTarget.DomainID
		resolution.Slug = linkTarget.Slug
		reportStatus = ReportStatusTakenDown
		action = "abuse.report.takedown"
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE abuse_reports
		SET status = ?, resolution_note = ?, resolved_by_user_id = ?,
			resolved_at = ?, updated_at = ?
		WHERE id = ?`,
		reportStatus, note, adminUserID, now, now, reportID,
	); err != nil {
		return nil, fmt.Errorf("更新举报处理结果失败: %w", err)
	}
	if err := insertAudit(
		ctx,
		tx,
		adminUserID,
		current.WorkspaceID,
		action,
		"abuse_report",
		reportID,
		map[string]any{
			"link_id":  current.LinkID,
			"decision": decision,
			"note":     note,
		},
		now,
	); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("提交举报处理事务失败: %w", err)
	}

	current.Status = reportStatus
	current.ResolutionNote = note
	current.ResolvedByUserID = &adminUserID
	current.ResolvedAt = &now
	current.UpdatedAt = now
	resolution.Report = current
	return resolution, nil
}

// ListBlockedDomains 返回当前有效的目标域名黑名单
func (r *MySQLRepository) ListBlockedDomains(ctx context.Context) ([]BlockedDomain, error) {
	domains := make([]BlockedDomain, 0)
	if err := r.db.SelectContext(ctx, &domains, `
		SELECT id, hostname, reason, created_by_user_id, deleted_at, created_at, updated_at
		FROM blocked_domains
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC, id DESC`); err != nil {
		return nil, fmt.Errorf("查询域名黑名单失败: %w", err)
	}
	return domains, nil
}

// AddBlockedDomain 新增或恢复域名规则并写入审计日志
func (r *MySQLRepository) AddBlockedDomain(
	ctx context.Context,
	domain *BlockedDomain,
) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始黑名单事务失败: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var existing struct {
		ID        string    `db:"id"`
		CreatedAt time.Time `db:"created_at"`
	}
	err = tx.GetContext(ctx, &existing, `
		SELECT id, created_at FROM blocked_domains WHERE hostname = ? FOR UPDATE`,
		domain.Hostname,
	)
	switch {
	case err == nil:
		if _, err := tx.ExecContext(ctx, `
			UPDATE blocked_domains
			SET reason = ?, created_by_user_id = ?, deleted_at = NULL, updated_at = ?
			WHERE id = ?`,
			domain.Reason, domain.CreatedByUserID, domain.UpdatedAt, existing.ID,
		); err != nil {
			return fmt.Errorf("恢复域名黑名单规则失败: %w", err)
		}
		domain.ID = existing.ID
		domain.CreatedAt = existing.CreatedAt
		domain.DeletedAt = nil
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO blocked_domains (
				id, hostname, reason, created_by_user_id, deleted_at, created_at, updated_at
			) VALUES (?, ?, ?, ?, NULL, ?, ?)`,
			domain.ID, domain.Hostname, domain.Reason, domain.CreatedByUserID,
			domain.CreatedAt, domain.UpdatedAt,
		); err != nil {
			var mysqlErr *driver.MySQLError
			if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
				return ErrAlreadyResolved
			}
			return fmt.Errorf("新增域名黑名单规则失败: %w", err)
		}
	default:
		return fmt.Errorf("查询域名黑名单规则失败: %w", err)
	}

	if err := insertAudit(
		ctx,
		tx,
		domain.CreatedByUserID,
		"",
		"abuse.blocked_domain.add",
		"blocked_domain",
		domain.Hostname,
		map[string]any{"reason": domain.Reason},
		domain.UpdatedAt,
	); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交黑名单事务失败: %w", err)
	}
	return nil
}

// RemoveBlockedDomain 停用域名规则并写入审计日志
func (r *MySQLRepository) RemoveBlockedDomain(
	ctx context.Context,
	id, adminUserID string,
	now time.Time,
) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始移除黑名单事务失败: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var hostname string
	if err := tx.GetContext(ctx, &hostname, `
		SELECT hostname FROM blocked_domains
		WHERE id = ? AND deleted_at IS NULL
		FOR UPDATE`, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("锁定域名黑名单规则失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE blocked_domains SET deleted_at = ?, updated_at = ? WHERE id = ?`,
		now, now, id,
	); err != nil {
		return fmt.Errorf("停用域名黑名单规则失败: %w", err)
	}
	if err := insertAudit(
		ctx,
		tx,
		adminUserID,
		"",
		"abuse.blocked_domain.remove",
		"blocked_domain",
		hostname,
		nil,
		now,
	); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交移除黑名单事务失败: %w", err)
	}
	return nil
}

// IsDestinationBlocked 判断目标 Host 是否命中有效黑名单规则
func (r *MySQLRepository) IsDestinationBlocked(
	ctx context.Context,
	hostname string,
) (bool, error) {
	var blocked bool
	if err := r.db.GetContext(ctx, &blocked, `
		SELECT EXISTS(
			SELECT 1 FROM blocked_domains
			WHERE deleted_at IS NULL
			  AND (? = hostname OR ? LIKE CONCAT('%.', hostname))
		)`, hostname, hostname); err != nil {
		return false, fmt.Errorf("检查目标域名黑名单失败: %w", err)
	}
	return blocked, nil
}

// insertAudit 在给定事务中保存管理员操作审计
func insertAudit(
	ctx context.Context,
	tx *sqlx.Tx,
	actorUserID, workspaceID, action, targetType, targetID string,
	metadata map[string]any,
	now time.Time,
) error {
	var metadataValue any
	if metadata != nil {
		payload, err := json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("序列化审计元数据失败: %w", err)
		}
		metadataValue = string(payload)
	}
	var nullableWorkspace any
	if workspaceID != "" {
		nullableWorkspace = workspaceID
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs (
			id, actor_user_id, workspace_id, action, target_type, target_id, metadata, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		ulid.Make().String(), actorUserID, nullableWorkspace, action,
		targetType, targetID, metadataValue, now,
	); err != nil {
		return fmt.Errorf("写入管理员审计日志失败: %w", err)
	}
	return nil
}
