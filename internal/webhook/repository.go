package webhook

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

const (
	// defaultDeliveryRetryBase 是首次失败后的退避基数
	defaultDeliveryRetryBase = 5 * time.Second
	// maxDeliveryRetryDelay 是单次失败后允许等待的最长时间
	maxDeliveryRetryDelay = 30 * time.Minute
)

// Repository 定义 Webhook 配置和 Outbox 投递的持久化能力
type Repository interface {
	// CreateEndpoint 保存新的 Webhook 端点
	CreateEndpoint(ctx context.Context, endpoint *Endpoint) error
	// ListEndpoints 返回工作空间未删除的端点
	ListEndpoints(ctx context.Context, workspaceID string) ([]Endpoint, error)
	// DisableEndpoint 停用端点并终止其待投递任务
	DisableEndpoint(ctx context.Context, workspaceID, id string, now time.Time) error
	// ListDeliveries 返回端点最近的投递记录
	ListDeliveries(ctx context.Context, workspaceID, endpointID string, limit int) ([]Delivery, error)
	// ClaimDeliveries 领取到期且未被其他 Worker 锁定的投递任务
	ClaimDeliveries(ctx context.Context, limit int, now, lockedUntil time.Time) ([]DeliveryJob, error)
	// MarkSuccess 将当前租约内的投递标记为成功
	MarkSuccess(
		ctx context.Context,
		job *DeliveryJob,
		responseStatus int,
		now time.Time,
	) error
	// MarkFailure 记录失败并按退避策略重试或终止投递
	MarkFailure(
		ctx context.Context,
		job *DeliveryJob,
		responseStatus *int,
		message string,
		now time.Time,
		maxAttempts int,
	) error
}

// MySQLRepository 使用 MySQL 保存 Webhook 配置和事务型 Outbox
type MySQLRepository struct {
	db *sqlx.DB
}

// NewMySQLRepository 创建 Webhook MySQL 仓库
func NewMySQLRepository(db *sqlx.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

// CreateEndpoint 保存新的 Webhook 端点
func (r *MySQLRepository) CreateEndpoint(ctx context.Context, endpoint *Endpoint) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO webhook_endpoints (
			id, workspace_id, name, url, secret_ciphertext, status,
			failure_count, disabled_at, deleted_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		endpoint.ID, endpoint.WorkspaceID, endpoint.Name, endpoint.URL,
		endpoint.SecretCiphertext, endpoint.Status, endpoint.FailureCount,
		endpoint.DisabledAt, endpoint.DeletedAt, endpoint.CreatedAt, endpoint.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("创建 Webhook 端点失败: %w", err)
	}
	return nil
}

// ListEndpoints 返回工作空间未删除的端点
func (r *MySQLRepository) ListEndpoints(ctx context.Context, workspaceID string) ([]Endpoint, error) {
	endpoints := make([]Endpoint, 0)
	if err := r.db.SelectContext(ctx, &endpoints, `
		SELECT id, workspace_id, name, url, status,
			failure_count, disabled_at, deleted_at, created_at, updated_at
		FROM webhook_endpoints
		WHERE workspace_id = ? AND deleted_at IS NULL
		ORDER BY created_at DESC, id DESC`, workspaceID); err != nil {
		return nil, fmt.Errorf("查询 Webhook 端点列表失败: %w", err)
	}
	return endpoints, nil
}

// DisableEndpoint 停用端点并终止其待投递任务
func (r *MySQLRepository) DisableEndpoint(
	ctx context.Context,
	workspaceID, id string,
	now time.Time,
) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始停用 Webhook 事务失败: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	result, err := tx.ExecContext(ctx, `
		UPDATE webhook_endpoints
		SET status = ?, disabled_at = COALESCE(disabled_at, ?), updated_at = ?
		WHERE workspace_id = ? AND id = ? AND deleted_at IS NULL`,
		EndpointStatusDisabled, now, now, workspaceID, id,
	)
	if err != nil {
		return fmt.Errorf("停用 Webhook 端点失败: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取 Webhook 停用行数失败: %w", err)
	}
	if affected == 0 {
		var exists int
		if err := tx.GetContext(ctx, &exists, `
			SELECT COUNT(*) FROM webhook_endpoints
			WHERE workspace_id = ? AND id = ? AND deleted_at IS NULL`,
			workspaceID, id,
		); err != nil {
			return fmt.Errorf("查询 Webhook 端点是否存在失败: %w", err)
		}
		if exists == 0 {
			return ErrNotFound
		}
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE webhook_deliveries
		SET status = ?, last_error = ?, updated_at = ?, locked_until = NULL
		WHERE webhook_id = ? AND status IN (?, ?)`,
		DeliveryStatusDead, "Webhook 端点已停用", now,
		id, DeliveryStatusPending, DeliveryStatusDelivering,
	); err != nil {
		return fmt.Errorf("终止 Webhook 待投递任务失败: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交 Webhook 停用事务失败: %w", err)
	}
	return nil
}

// ListDeliveries 返回端点最近的投递记录
func (r *MySQLRepository) ListDeliveries(
	ctx context.Context,
	workspaceID, endpointID string,
	limit int,
) ([]Delivery, error) {
	var exists int
	if err := r.db.GetContext(ctx, &exists, `
		SELECT COUNT(*) FROM webhook_endpoints
		WHERE workspace_id = ? AND id = ? AND deleted_at IS NULL`,
		workspaceID, endpointID,
	); err != nil {
		return nil, fmt.Errorf("查询 Webhook 端点是否存在失败: %w", err)
	}
	if exists == 0 {
		return nil, ErrNotFound
	}

	deliveries := make([]Delivery, 0, limit)
	if err := r.db.SelectContext(ctx, &deliveries, `
		SELECT id, workspace_id, webhook_id, event_id, event_type, payload,
			status, attempt_count, next_attempt_at, locked_until,
			response_status, last_error, delivered_at, created_at, updated_at
		FROM webhook_deliveries
		WHERE workspace_id = ? AND webhook_id = ?
		ORDER BY id DESC
		LIMIT ?`, workspaceID, endpointID, limit); err != nil {
		return nil, fmt.Errorf("查询 Webhook 投递记录失败: %w", err)
	}
	return deliveries, nil
}

// ClaimDeliveries 领取到期且未被其他 Worker 锁定的投递任务
func (r *MySQLRepository) ClaimDeliveries(
	ctx context.Context,
	limit int,
	now, lockedUntil time.Time,
) ([]DeliveryJob, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("开始领取 Webhook 任务事务失败: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	ids := make([]uint64, 0, limit)
	if err := tx.SelectContext(ctx, &ids, `
		SELECT id
		FROM webhook_deliveries
		WHERE status = ?
		  AND next_attempt_at <= ?
		  AND (locked_until IS NULL OR locked_until < ?)
		ORDER BY next_attempt_at ASC, id ASC
		LIMIT ?
		FOR UPDATE SKIP LOCKED`,
		DeliveryStatusPending, now, now, limit,
	); err != nil {
		return nil, fmt.Errorf("查询待投递 Webhook 任务失败: %w", err)
	}
	if len(ids) == 0 {
		return []DeliveryJob{}, nil
	}

	placeholders, idArgs := uint64Placeholders(ids)
	args := []any{DeliveryStatusDelivering, lockedUntil, now, DeliveryStatusPending}
	args = append(args, idArgs...)
	query := fmt.Sprintf(`
		UPDATE webhook_deliveries
		SET status = ?, locked_until = ?, attempt_count = attempt_count + 1, updated_at = ?
		WHERE status = ? AND id IN (%s)`, placeholders)
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return nil, fmt.Errorf("锁定待投递 Webhook 任务失败: %w", err)
	}

	jobs := make([]DeliveryJob, 0, len(ids))
	selectQuery := fmt.Sprintf(`
		SELECT d.id, d.workspace_id, d.webhook_id, d.event_id, d.event_type,
			d.payload, d.status, d.attempt_count, d.next_attempt_at, d.locked_until,
			d.response_status, d.last_error, d.delivered_at, d.created_at, d.updated_at,
			e.url AS endpoint_url, e.secret_ciphertext
		FROM webhook_deliveries AS d
		INNER JOIN webhook_endpoints AS e ON e.id = d.webhook_id
		WHERE d.id IN (%s)
		  AND e.status = ?
		  AND e.deleted_at IS NULL
		ORDER BY d.id ASC`, placeholders)
	selectArgs := append([]any{}, idsToInterfaces(ids)...)
	selectArgs = append(selectArgs, EndpointStatusActive)
	if err := tx.SelectContext(ctx, &jobs, selectQuery, selectArgs...); err != nil {
		return nil, fmt.Errorf("读取已领取 Webhook 任务失败: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("提交 Webhook 领取事务失败: %w", err)
	}
	return jobs, nil
}

// MarkSuccess 将当前租约内的投递标记为成功
func (r *MySQLRepository) MarkSuccess(
	ctx context.Context,
	job *DeliveryJob,
	responseStatus int,
	now time.Time,
) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始 Webhook 成功事务失败: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	result, err := tx.ExecContext(ctx, `
		UPDATE webhook_deliveries
		SET status = ?, response_status = ?, last_error = '', locked_until = NULL,
			delivered_at = ?, updated_at = ?
		WHERE id = ? AND status = ? AND locked_until = ?`,
		DeliveryStatusSuccess, responseStatus, now, now,
		job.ID, DeliveryStatusDelivering, job.LockedUntil,
	)
	if err != nil {
		return fmt.Errorf("标记 Webhook 投递成功失败: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取 Webhook 成功行数失败: %w", err)
	}
	if affected == 0 {
		return ErrLeaseLost
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE webhook_endpoints
		SET failure_count = 0, updated_at = ?
		WHERE id = ? AND status = ? AND deleted_at IS NULL`,
		now, job.WebhookID, EndpointStatusActive,
	); err != nil {
		return fmt.Errorf("重置 Webhook 连续失败次数失败: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交 Webhook 成功事务失败: %w", err)
	}
	return nil
}

// MarkFailure 记录失败并按退避策略重试或终止投递
func (r *MySQLRepository) MarkFailure(
	ctx context.Context,
	job *DeliveryJob,
	responseStatus *int,
	message string,
	now time.Time,
	maxAttempts int,
) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始 Webhook 失败事务失败: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	dead := job.AttemptCount >= maxAttempts
	status := DeliveryStatusPending
	nextAttemptAt := now.Add(deliveryRetryDelay(job.AttemptCount))
	if dead {
		status = DeliveryStatusDead
		nextAttemptAt = now
	}
	message = truncateErrorMessage(message)
	result, err := tx.ExecContext(ctx, `
		UPDATE webhook_deliveries
		SET status = ?, response_status = ?, last_error = ?, next_attempt_at = ?,
			locked_until = NULL, updated_at = ?
		WHERE id = ? AND status = ? AND locked_until = ?`,
		status, responseStatus, message, nextAttemptAt, now,
		job.ID, DeliveryStatusDelivering, job.LockedUntil,
	)
	if err != nil {
		return fmt.Errorf("记录 Webhook 投递失败失败: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取 Webhook 失败行数失败: %w", err)
	}
	if affected == 0 {
		return ErrLeaseLost
	}

	endpointResult, err := tx.ExecContext(ctx, `
		UPDATE webhook_endpoints
		SET failure_count = failure_count + 1, updated_at = ?
		WHERE id = ? AND status = ? AND deleted_at IS NULL`,
		now, job.WebhookID, EndpointStatusActive,
	)
	if err != nil {
		return fmt.Errorf("更新 Webhook 连续失败次数失败: %w", err)
	}
	endpointAffected, err := endpointResult.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取 Webhook 端点失败行数失败: %w", err)
	}
	if dead && endpointAffected > 0 {
		if err := disableFailedEndpoint(ctx, tx, job.WebhookID, now, message); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交 Webhook 失败事务失败: %w", err)
	}
	return nil
}

// disableFailedEndpoint 在失败事务内停用端点并终止其余待投递任务
func disableFailedEndpoint(
	ctx context.Context,
	tx *sqlx.Tx,
	endpointID string,
	now time.Time,
	message string,
) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE webhook_endpoints
		SET status = ?, disabled_at = COALESCE(disabled_at, ?), updated_at = ?
		WHERE id = ? AND status = ?`,
		EndpointStatusDisabled, now, now, endpointID, EndpointStatusActive,
	); err != nil {
		return fmt.Errorf("自动停用 Webhook 端点失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE webhook_deliveries
		SET status = ?, last_error = ?, locked_until = NULL, updated_at = ?
		WHERE webhook_id = ? AND status IN (?, ?)`,
		DeliveryStatusDead,
		truncateErrorMessage("Webhook 端点连续失败已自动停用: "+message),
		now, endpointID, DeliveryStatusPending, DeliveryStatusDelivering,
	); err != nil {
		return fmt.Errorf("终止自动停用端点的待投递任务失败: %w", err)
	}
	return nil
}

// deliveryRetryDelay 根据已尝试次数计算有上限的指数退避时间
func deliveryRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	shift := min(attempt-1, 9)
	delay := defaultDeliveryRetryBase * time.Duration(1<<shift)
	if delay > maxDeliveryRetryDelay {
		return maxDeliveryRetryDelay
	}
	return delay
}

// uint64Placeholders 为 ID 列表生成 SQL 占位符
func uint64Placeholders(ids []uint64) (string, []any) {
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for index, id := range ids {
		placeholders[index] = "?"
		args[index] = id
	}
	return strings.Join(placeholders, ","), args
}

// idsToInterfaces 将 ID 列表转换为 SQL 查询参数
func idsToInterfaces(ids []uint64) []any {
	args := make([]any, len(ids))
	for index, id := range ids {
		args[index] = id
	}
	return args
}

// truncateErrorMessage 限制持久化错误文本长度，避免超出列宽
func truncateErrorMessage(message string) string {
	const maxRunes = 500
	runes := []rune(strings.TrimSpace(message))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes]) + "…"
}
