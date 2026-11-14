package link

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

const (
	// maxBulkRows 是单个 CSV 文件允许创建的最大数据行数
	maxBulkRows = 500
	// maxBulkColumns 是 CSV 表头允许的最大列数
	maxBulkColumns = 7
	// maxBulkCSVBytes 是单个 CSV 文件内容的最大字节数
	maxBulkCSVBytes = 900 * 1024
)

var errBulkFileTooLarge = errors.New("CSV 文件过大")

var bulkHeaders = []string{
	"destination_url",
	"alias",
	"title",
	"notes",
	"redirect_code",
	"starts_at",
	"expires_at",
}

// BulkCreateItem 是 CSV 单行的创建结果
type BulkCreateItem struct {
	// Row 是 CSV 数据行号，表头为第 1 行
	Row int
	// Status 是 created 或 failed
	Status string
	// Link 是成功创建的短链
	Link *Link
	// ErrorCode 是失败行的稳定错误码
	ErrorCode string
	// ErrorMessage 是失败行可展示的错误信息
	ErrorMessage string
}

// BulkCreateResult 是一次同步 CSV 批次的逐行结果
type BulkCreateResult struct {
	// BatchID 是本次导入的 ULID，用于日志关联
	BatchID string
	// Total 是数据行总数
	Total int
	// Succeeded 是成功创建数量
	Succeeded int
	// Failed 是失败数量
	Failed int
	// Items 是按 CSV 行号排列的结果
	Items []BulkCreateItem
}

type bulkRawRow struct {
	// row 是 CSV 数据行号
	row int
	// values 是按表头顺序排列的字段
	values []string
}

// BulkCreate 解析 CSV 并逐行创建短链，合法行不会因其他行失败而回滚
func (s *Service) BulkCreate(
	ctx context.Context,
	workspaceID string,
	reader io.Reader,
	idempotencyKeys ...string,
) (*BulkCreateResult, error) {
	header, rows, err := parseBulkCSV(reader)
	if err != nil {
		return nil, err
	}
	batchKey := ""
	if len(idempotencyKeys) > 0 {
		batchKey = strings.TrimSpace(idempotencyKeys[0])
	}

	result := &BulkCreateResult{
		BatchID: ulid.Make().String(),
		Total:   len(rows),
		Items:   make([]BulkCreateItem, 0, len(rows)),
	}
	for _, raw := range rows {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		item := s.createBulkRow(ctx, workspaceID, header, raw, batchKey)
		if item.Status == "created" {
			result.Succeeded++
		} else {
			result.Failed++
		}
		result.Items = append(result.Items, item)
	}
	return result, nil
}

// createBulkRow 校验单行字段并复用正式短链创建服务
func (s *Service) createBulkRow(
	ctx context.Context,
	workspaceID string,
	header map[string]int,
	raw bulkRawRow,
	batchKey string,
) BulkCreateItem {
	item := BulkCreateItem{Row: raw.row, Status: "failed"}
	if len(raw.values) != len(header) {
		item.ErrorCode = "INVALID_COLUMN_COUNT"
		item.ErrorMessage = "该行列数与表头不一致"
		return item
	}
	field := func(name string) string {
		index, ok := header[name]
		if !ok || index >= len(raw.values) {
			return ""
		}
		return strings.TrimSpace(raw.values[index])
	}

	var redirectCode int
	if value := field("redirect_code"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			item.ErrorCode = "INVALID_REDIRECT_CODE"
			item.ErrorMessage = "redirect_code 必须是 301 或 302"
			return item
		}
		redirectCode = parsed
	}
	startsAt, err := parseOptionalBulkTime(field("starts_at"))
	if err != nil {
		item.ErrorCode = "INVALID_START_TIME"
		item.ErrorMessage = "starts_at 必须使用 RFC3339 时间"
		return item
	}
	expiresAt, err := parseOptionalBulkTime(field("expires_at"))
	if err != nil {
		item.ErrorCode = "INVALID_EXPIRY_TIME"
		item.ErrorMessage = "expires_at 必须使用 RFC3339 时间"
		return item
	}

	created, err := s.Create(ctx, workspaceID, CreateInput{
		DestinationURL: field("destination_url"),
		Alias:          field("alias"),
		Title:          field("title"),
		Notes:          field("notes"),
		RedirectCode:   redirectCode,
		StartsAt:       startsAt,
		ExpiresAt:      expiresAt,
		IdempotencyKey: bulkRowIdempotencyKey(batchKey, raw.row),
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrSlugTaken):
			item.ErrorCode = "SLUG_TAKEN"
			item.ErrorMessage = "短码已被占用"
		case errors.Is(err, ErrInvalidInput):
			item.ErrorCode = "INVALID_INPUT"
			item.ErrorMessage = err.Error()
		default:
			item.ErrorCode = "INTERNAL_ERROR"
			item.ErrorMessage = "创建该行失败"
		}
		return item
	}
	item.Status = "created"
	item.Link = created
	return item
}

// bulkRowIdempotencyKey 根据批次幂等键和 CSV 行号生成稳定行级键。
func bulkRowIdempotencyKey(batchKey string, row int) string {
	if batchKey == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", batchKey, row)))
	return hex.EncodeToString(sum[:])
}

// parseBulkCSV 先完整解析文件结构，文件级错误会阻止任何行写入
func parseBulkCSV(reader io.Reader) (map[string]int, []bulkRawRow, error) {
	csvReader := csv.NewReader(reader)
	csvReader.FieldsPerRecord = -1
	csvReader.TrimLeadingSpace = true
	csvReader.ReuseRecord = false

	headerRecord, err := csvReader.Read()
	if errors.Is(err, io.EOF) {
		return nil, nil, fmt.Errorf("%w: CSV 文件为空", ErrInvalidInput)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("%w: 读取 CSV 表头失败", ErrInvalidInput)
	}
	if len(headerRecord) == 0 || len(headerRecord) > maxBulkColumns {
		return nil, nil, fmt.Errorf("%w: CSV 列数无效", ErrInvalidInput)
	}

	allowed := make(map[string]struct{}, len(bulkHeaders))
	for _, name := range bulkHeaders {
		allowed[name] = struct{}{}
	}
	header := make(map[string]int, len(headerRecord))
	for index, name := range headerRecord {
		name = strings.TrimSpace(strings.TrimPrefix(name, "\uFEFF"))
		if _, ok := allowed[name]; !ok {
			return nil, nil, fmt.Errorf("%w: 不支持的表头 %q", ErrInvalidInput, name)
		}
		if _, duplicate := header[name]; duplicate {
			return nil, nil, fmt.Errorf("%w: 表头 %q 重复", ErrInvalidInput, name)
		}
		header[name] = index
	}
	if _, ok := header["destination_url"]; !ok {
		return nil, nil, fmt.Errorf("%w: 缺少 destination_url 表头", ErrInvalidInput)
	}

	rows := make([]bulkRawRow, 0)
	for rowNumber := 2; ; rowNumber++ {
		record, readErr := csvReader.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, nil, fmt.Errorf("%w: 第 %d 行 CSV 格式错误", ErrInvalidInput, rowNumber)
		}
		if len(rows) >= maxBulkRows {
			return nil, nil, fmt.Errorf("%w: CSV 最多允许 %d 行", ErrInvalidInput, maxBulkRows)
		}
		rows = append(rows, bulkRawRow{row: rowNumber, values: record})
	}
	if len(rows) == 0 {
		return nil, nil, fmt.Errorf("%w: CSV 没有数据行", ErrInvalidInput)
	}
	return header, rows, nil
}

// parseOptionalBulkTime 解析可选 RFC3339 时间并转换为 UTC
func parseOptionalBulkTime(value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, err
	}
	utc := parsed.UTC()
	return &utc, nil
}
