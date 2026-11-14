package link

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	qrcode "github.com/skip2/go-qrcode"

	"github.com/PYang176/Short-URL-Service/internal/identity"
	"github.com/PYang176/Short-URL-Service/internal/platform/idempotency"
)

// Handler 提供短链接管理和公开跳转 HTTP 接口
type Handler struct {
	service *Service
	// recorder 记录成功跳转产生的点击事件
	recorder ClickRecorder
	// logger 记录点击写入失败等非致命错误
	logger *slog.Logger
	// idempotency 保存创建短链的首次响应
	idempotency *idempotency.Service
}

// NewHandler 创建短链接 HTTP 处理器
func NewHandler(
	service *Service,
	recorder ClickRecorder,
	idempotencyService *idempotency.Service,
	logger *slog.Logger,
) *Handler {
	return &Handler{
		service:     service,
		recorder:    recorder,
		idempotency: idempotencyService,
		logger:      logger,
	}
}

type createRequest struct {
	// DestinationURL 是准备缩短的目标地址
	DestinationURL string `json:"destination_url" binding:"required"`
	// DomainID 是可选自定义域名 ID
	DomainID string `json:"domain_id"`
	// Alias 是可选自定义短码
	Alias string `json:"alias"`
	// Title 是可选链接标题
	Title string `json:"title"`
	// Notes 是可选备注
	Notes string `json:"notes"`
	// RedirectCode 是 301 或 302，默认 302
	RedirectCode int `json:"redirect_code"`
	// StartsAt 是可选生效时间
	StartsAt *time.Time `json:"starts_at"`
	// ExpiresAt 是可选过期时间
	ExpiresAt *time.Time `json:"expires_at"`
}

type updateRequest struct {
	// DestinationURL 是可选的新目标地址
	DestinationURL *string `json:"destination_url"`
	// Title 是可选的新标题
	Title *string `json:"title"`
	// Notes 是可选的新备注
	Notes *string `json:"notes"`
	// Status 是可选的新状态
	Status *uint8 `json:"status"`
	// RedirectCode 是可选的新跳转状态码
	RedirectCode *int `json:"redirect_code"`
	// ExpiresAt 是可选的新过期时间
	ExpiresAt *time.Time `json:"expires_at"`
	// ClearExpiresAt 表示删除已有过期时间
	ClearExpiresAt bool `json:"clear_expires_at"`
}

type linkResponse struct {
	// ID 是 ULID 链接标识
	ID string `json:"id"`
	// Slug 是大小写敏感短码
	Slug string `json:"slug"`
	// ShortURL 是完整短链地址
	ShortURL string `json:"short_url"`
	// DomainID 是链接所用域名 ID
	DomainID string `json:"domain_id"`
	// DomainHostname 是链接所用域名 Host
	DomainHostname string `json:"domain_hostname"`
	// DestinationURL 是当前目标地址
	DestinationURL string `json:"destination_url"`
	// Title 是链接标题
	Title string `json:"title"`
	// Notes 是链接备注
	Notes string `json:"notes"`
	// Status 是链接状态
	Status uint8 `json:"status"`
	// RedirectCode 是 301 或 302
	RedirectCode int `json:"redirect_code"`
	// StartsAt 是可选生效时间
	StartsAt *time.Time `json:"starts_at"`
	// ExpiresAt 是可选过期时间
	ExpiresAt *time.Time `json:"expires_at"`
	// CreatedAt 是链接创建时间
	CreatedAt time.Time `json:"created_at"`
}

type listResponse struct {
	// Items 是当前页链接
	Items []linkResponse `json:"items"`
	// NextCursor 是下一页游标，空值表示没有更多数据
	NextCursor string `json:"next_cursor"`
}

type bulkCreateItemResponse struct {
	// Row 是 CSV 数据行号
	Row int `json:"row"`
	// Status 是 created 或 failed
	Status string `json:"status"`
	// Link 是成功创建的短链
	Link *linkResponse `json:"link,omitempty"`
	// ErrorCode 是失败行稳定错误码
	ErrorCode string `json:"error_code,omitempty"`
	// ErrorMessage 是失败行错误信息
	ErrorMessage string `json:"error_message,omitempty"`
}

type bulkCreateResponse struct {
	// BatchID 是本次导入 ULID
	BatchID string `json:"batch_id"`
	// Total 是数据行总数
	Total int `json:"total"`
	// Succeeded 是成功数量
	Succeeded int `json:"succeeded"`
	// Failed 是失败数量
	Failed int `json:"failed"`
	// Items 是逐行结果
	Items []bulkCreateItemResponse `json:"items"`
}

// QRCode 生成指向稳定短链地址的动态二维码
func (h *Handler) QRCode(c *gin.Context) {
	link, err := h.service.Get(
		c.Request.Context(),
		c.GetString(identity.ContextWorkspaceID),
		c.Param("id"),
	)
	if err != nil {
		writeLinkError(c, err)
		return
	}

	size := 512
	if rawSize := c.Query("size"); rawSize != "" {
		parsed, err := strconv.Atoi(rawSize)
		if err != nil || parsed < 128 || parsed > 1024 {
			writeError(c, http.StatusBadRequest, "INVALID_QR_SIZE", "二维码尺寸必须为 128 至 1024")
			return
		}
		size = parsed
	}

	payload, err := qrPayload(h.service.ShortURL(link))
	if err != nil {
		h.logger.Error("生成二维码内容失败", "link_id", link.ID, "error", err)
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "服务器内部错误")
		return
	}
	code, err := qrcode.New(payload, qrcode.Medium)
	if err != nil {
		h.logger.Error("生成二维码失败", "link_id", link.ID, "error", err)
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "服务器内部错误")
		return
	}
	code.DisableBorder = false

	format := strings.ToLower(c.DefaultQuery("format", "png"))
	c.Header("Cache-Control", "private, max-age=300")
	switch format {
	case "png":
		image, err := code.PNG(size)
		if err != nil {
			h.logger.Error("编码二维码 PNG 失败", "link_id", link.ID, "error", err)
			writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "服务器内部错误")
			return
		}
		c.Header("Content-Type", "image/png")
		c.Header("Content-Disposition", `inline; filename="`+link.Slug+`.png"`)
		c.Data(http.StatusOK, "image/png", image)
	case "svg":
		image := qrSVG(code, size)
		c.Header("Content-Type", "image/svg+xml")
		c.Header("Content-Disposition", `inline; filename="`+link.Slug+`.svg"`)
		c.Data(http.StatusOK, "image/svg+xml", []byte(image))
	default:
		writeError(c, http.StatusBadRequest, "INVALID_QR_FORMAT", "二维码格式只支持 png 或 svg")
	}
}

// Create 创建当前工作空间下的短链接
func (h *Handler) Create(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "读取请求体失败")
		return
	}
	var request createRequest
	if err := decodeJSON(body, &request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "创建短链请求参数无效")
		return
	}

	workspaceID := c.GetString(identity.ContextWorkspaceID)
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	var requestHash []byte
	if key != "" && h.idempotency != nil {
		if !validIdempotencyKey(key) {
			writeError(c, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key 格式无效")
			return
		}
		sum := sha256.Sum256(append([]byte("POST:/api/v1/links:"), body...))
		requestHash = sum[:]
		record, claimed, beginErr := h.idempotency.Begin(c.Request.Context(), workspaceID, key, requestHash)
		if beginErr != nil {
			switch {
			case errors.Is(beginErr, idempotency.ErrRequestMismatch):
				writeError(c, http.StatusConflict, "IDEMPOTENCY_MISMATCH", "幂等键已用于其他请求")
			case errors.Is(beginErr, idempotency.ErrInProgress):
				c.Header("Retry-After", "1")
				writeError(c, http.StatusConflict, "IDEMPOTENCY_IN_PROGRESS", "相同请求正在处理中")
			default:
				h.logger.Error("处理幂等键失败", "error", beginErr)
				writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "服务器内部错误")
			}
			return
		}
		if !claimed {
			if record.State != idempotency.StateCompleted ||
				!record.StatusCode.Valid || !record.ResponseBody.Valid {
				c.Header("Retry-After", "1")
				writeError(c, http.StatusConflict, "IDEMPOTENCY_IN_PROGRESS", "相同请求正在处理中")
				return
			}
			c.Header("Idempotent-Replayed", "true")
			c.Data(int(record.StatusCode.Int64), "application/json; charset=utf-8", []byte(record.ResponseBody.String))
			return
		}
	}

	link, createErr := h.service.Create(
		c.Request.Context(),
		workspaceID,
		CreateInput(request),
	)
	if createErr != nil {
		if key != "" && h.idempotency != nil {
			if abortErr := h.idempotency.Abort(c.Request.Context(), workspaceID, key, requestHash); abortErr != nil {
				h.logger.Warn("释放幂等键失败", "error", abortErr)
			}
		}
		writeLinkError(c, createErr)
		return
	}
	responseBody, err := json.Marshal(h.toResponse(link))
	if err != nil {
		h.logger.Error("序列化创建短链响应失败", "error", err)
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "服务器内部错误")
		return
	}
	if key != "" && h.idempotency != nil {
		if err := h.idempotency.Complete(
			c.Request.Context(), workspaceID, key, requestHash,
			http.StatusCreated, string(responseBody),
		); err != nil {
			h.logger.Error("保存幂等响应失败", "error", err)
		}
	}
	c.Data(http.StatusCreated, "application/json; charset=utf-8", responseBody)
}

// BulkCreate 解析 CSV 文件并返回逐行创建结果
func (h *Handler) BulkCreate(c *gin.Context) {
	csvBody, err := readBulkCSV(c.Request)
	if err != nil {
		switch {
		case errors.Is(err, errBulkFileTooLarge):
			writeError(c, http.StatusRequestEntityTooLarge, "BULK_FILE_TOO_LARGE", "CSV 文件不能超过 900 KiB")
		case errors.Is(err, ErrInvalidInput):
			writeError(c, http.StatusBadRequest, "INVALID_BULK_UPLOAD", err.Error())
		default:
			writeError(c, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "请上传 multipart/form-data CSV 文件")
		}
		return
	}

	workspaceID := c.GetString(identity.ContextWorkspaceID)
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	var requestHash []byte
	if key != "" && h.idempotency != nil {
		if !validIdempotencyKey(key) {
			writeError(c, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key 格式无效")
			return
		}
		sum := sha256.Sum256(append([]byte("POST:/api/v1/links/bulk:"), csvBody...))
		requestHash = sum[:]
		record, claimed, beginErr := h.idempotency.Begin(c.Request.Context(), workspaceID, key, requestHash)
		if beginErr != nil {
			switch {
			case errors.Is(beginErr, idempotency.ErrRequestMismatch):
				writeError(c, http.StatusConflict, "IDEMPOTENCY_MISMATCH", "幂等键已用于其他请求")
			case errors.Is(beginErr, idempotency.ErrInProgress):
				c.Header("Retry-After", "1")
				writeError(c, http.StatusConflict, "IDEMPOTENCY_IN_PROGRESS", "相同请求正在处理中")
			default:
				h.logger.Error("处理批量创建幂等键失败", "error", beginErr)
				writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "服务器内部错误")
			}
			return
		}
		if !claimed {
			if record.State != idempotency.StateCompleted ||
				!record.StatusCode.Valid || !record.ResponseBody.Valid {
				c.Header("Retry-After", "1")
				writeError(c, http.StatusConflict, "IDEMPOTENCY_IN_PROGRESS", "相同请求正在处理中")
				return
			}
			c.Header("Idempotent-Replayed", "true")
			c.Data(int(record.StatusCode.Int64), "application/json; charset=utf-8", []byte(record.ResponseBody.String))
			return
		}
	}

	result, createErr := h.service.BulkCreate(
		c.Request.Context(),
		workspaceID,
		bytes.NewReader(csvBody),
	)
	if createErr != nil {
		if key != "" && h.idempotency != nil {
			if abortErr := h.idempotency.Abort(c.Request.Context(), workspaceID, key, requestHash); abortErr != nil {
				h.logger.Warn("释放批量创建幂等键失败", "error", abortErr)
			}
		}
		writeLinkError(c, createErr)
		return
	}
	response := h.toBulkResponse(result)
	responseBody, err := json.Marshal(response)
	if err != nil {
		h.logger.Error("序列化批量创建响应失败", "error", err)
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "服务器内部错误")
		return
	}
	if key != "" && h.idempotency != nil {
		if err := h.idempotency.Complete(
			c.Request.Context(),
			workspaceID,
			key,
			requestHash,
			http.StatusOK,
			string(responseBody),
		); err != nil {
			h.logger.Error("保存批量创建幂等响应失败", "error", err)
		}
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", responseBody)
}

// List 返回当前工作空间的短链接列表
func (h *Handler) List(c *gin.Context) {
	limit := 20
	if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil {
			writeError(c, http.StatusBadRequest, "INVALID_CURSOR", "limit 必须为数字")
			return
		}
		limit = parsed
	}
	beforeCreatedAt, beforeID, err := decodeCursor(c.Query("cursor"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_CURSOR", "分页游标无效")
		return
	}

	links, err := h.service.List(
		c.Request.Context(),
		c.GetString(identity.ContextWorkspaceID),
		limit,
		beforeCreatedAt,
		beforeID,
	)
	if err != nil {
		writeLinkError(c, err)
		return
	}

	items := make([]linkResponse, 0, len(links))
	for index := range links {
		items = append(items, h.toResponse(&links[index]))
	}
	nextCursor := ""
	if len(links) == limit && len(links) > 0 {
		nextCursor = encodeCursor(links[len(links)-1])
	}
	c.JSON(http.StatusOK, listResponse{Items: items, NextCursor: nextCursor})
}

// Get 返回当前工作空间内的单个短链接
func (h *Handler) Get(c *gin.Context) {
	link, err := h.service.Get(
		c.Request.Context(),
		c.GetString(identity.ContextWorkspaceID),
		c.Param("id"),
	)
	if err != nil {
		writeLinkError(c, err)
		return
	}
	c.JSON(http.StatusOK, h.toResponse(link))
}

// Update 修改当前工作空间内的短链接
func (h *Handler) Update(c *gin.Context) {
	var request updateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "更新短链请求参数无效")
		return
	}
	link, err := h.service.Update(
		c.Request.Context(),
		c.GetString(identity.ContextWorkspaceID),
		c.Param("id"),
		UpdateInput(request),
	)
	if err != nil {
		writeLinkError(c, err)
		return
	}
	c.JSON(http.StatusOK, h.toResponse(link))
}

// Delete 软删除当前工作空间内的短链接
func (h *Handler) Delete(c *gin.Context) {
	if err := h.service.Delete(
		c.Request.Context(),
		c.GetString(identity.ContextWorkspaceID),
		c.Param("id"),
	); err != nil {
		writeLinkError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// Redirect 解析短码并返回 HTTP 跳转
func (h *Handler) Redirect(c *gin.Context) {
	result, err := h.service.Resolve(c.Request.Context(), c.Param("slug"))
	if err != nil {
		writeLinkError(c, err)
		return
	}
	h.recordClick(c, result)
	http.Redirect(c.Writer, c.Request, result.DestinationURL, result.RedirectCode)
}

// RedirectByHost 按请求 Host 解析自定义域名短链
func (h *Handler) RedirectByHost(c *gin.Context) {
	hostname := c.Request.Host
	if host, _, err := net.SplitHostPort(hostname); err == nil {
		hostname = host
	}
	result, err := h.service.ResolveByHost(c.Request.Context(), hostname, c.Param("slug"))
	if err != nil {
		writeLinkError(c, err)
		return
	}
	h.recordClick(c, result)
	http.Redirect(c.Writer, c.Request, result.DestinationURL, result.RedirectCode)
}

// recordClick 记录一次成功跳转，失败不阻断响应
func (h *Handler) recordClick(c *gin.Context, result ResolveResult) {
	if h.recorder == nil {
		return
	}
	recordCtx, cancel := context.WithTimeout(c.Request.Context(), 100*time.Millisecond)
	defer cancel()
	if err := h.recorder.RecordClick(recordCtx, ClickContext{
		ResolveResult: result,
		IP:            c.ClientIP(),
		UserAgent:     c.Request.UserAgent(),
		Referrer:      c.Request.Referer(),
		Language:      c.GetHeader("Accept-Language"),
		Country:       c.GetHeader("CF-IPCountry"),
		Query:         c.Request.URL.Query(),
	}); err != nil {
		h.logger.Warn("写入点击事件失败", "link_id", result.LinkID, "error", err)
	}
}

// toResponse 将链接模型转换为 HTTP DTO
func (h *Handler) toResponse(link *Link) linkResponse {
	return linkResponse{
		ID:             link.ID,
		Slug:           link.Slug,
		ShortURL:       h.service.ShortURL(link),
		DomainID:       link.DomainID,
		DomainHostname: link.DomainHostname,
		DestinationURL: link.DestinationURL,
		Title:          link.Title,
		Notes:          link.Notes,
		Status:         link.Status,
		RedirectCode:   link.RedirectCode,
		StartsAt:       link.StartsAt,
		ExpiresAt:      link.ExpiresAt,
		CreatedAt:      link.CreatedAt,
	}
}

// toBulkResponse 将批量业务结果转换为 HTTP DTO
func (h *Handler) toBulkResponse(result *BulkCreateResult) bulkCreateResponse {
	items := make([]bulkCreateItemResponse, 0, len(result.Items))
	for index := range result.Items {
		item := &result.Items[index]
		response := bulkCreateItemResponse{
			Row:          item.Row,
			Status:       item.Status,
			ErrorCode:    item.ErrorCode,
			ErrorMessage: item.ErrorMessage,
		}
		if item.Link != nil {
			linkResponse := h.toResponse(item.Link)
			response.Link = &linkResponse
		}
		items = append(items, response)
	}
	return bulkCreateResponse{
		BatchID:   result.BatchID,
		Total:     result.Total,
		Succeeded: result.Succeeded,
		Failed:    result.Failed,
		Items:     items,
	}
}

// readBulkCSV 从 multipart 流读取唯一 CSV 文件并限制文件大小
func readBulkCSV(request *http.Request) ([]byte, error) {
	reader, err := request.MultipartReader()
	if err != nil {
		return nil, err
	}
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("%w: 缺少 file 文件字段", ErrInvalidInput)
		}
		if err != nil {
			return nil, err
		}
		if part.FormName() != "file" {
			_ = part.Close()
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(part, maxBulkCSVBytes+1))
		closeErr := part.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if len(body) > maxBulkCSVBytes {
			return nil, errBulkFileTooLarge
		}
		return body, nil
	}
}

// encodeCursor 将链接创建时间和 ID 编码为不透明分页游标
func encodeCursor(link Link) string {
	value := link.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + link.ID
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

// decodeCursor 解析分页游标并返回创建时间和链接 ID
func decodeCursor(cursor string) (*time.Time, string, error) {
	if strings.TrimSpace(cursor) == "" {
		return nil, "", nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, "", err
	}
	parts := strings.SplitN(string(decoded), "|", 2)
	if len(parts) != 2 || parts[1] == "" {
		return nil, "", ErrInvalidInput
	}
	createdAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, "", err
	}
	return &createdAt, parts[1], nil
}

// qrPayload 为短链追加二维码来源标记，便于后续分析扫码流量
func qrPayload(shortURL string) (string, error) {
	parsed, err := url.Parse(shortURL)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("utm_source", "qr")
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

// qrSVG 根据二维码矩阵生成带静区的简单 SVG
func qrSVG(code *qrcode.QRCode, size int) []byte {
	bitmap := code.Bitmap()
	modules := len(bitmap)
	var path strings.Builder
	for y, row := range bitmap {
		for x := 0; x < len(row); {
			if !row[x] {
				x++
				continue
			}
			start := x
			for x < len(row) && row[x] {
				x++
			}
			fmt.Fprintf(&path, "M%d %dh%dv1h-%dz", start, y, x-start, x-start)
		}
	}

	svg := fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" shape-rendering="crispEdges"><rect width="100%%" height="100%%" fill="#fff"/><path d="%s" fill="#000"/></svg>`,
		size, size, modules, modules, path.String(),
	)
	return []byte(svg)
}

// decodeJSON 严格解析 JSON 并拒绝未知字段
func decodeJSON(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

// validIdempotencyKey 校验客户端幂等键的字符和长度
func validIdempotencyKey(value string) bool {
	if len(value) < 8 || len(value) > 128 {
		return false
	}
	for _, character := range value {
		valid := character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			character == '-' || character == '_' ||
			character == '.' || character == ':'
		if !valid {
			return false
		}
	}
	return true
}

// writeLinkError 将短链领域错误映射为 HTTP 响应
func writeLinkError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		writeError(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
	case errors.Is(err, ErrSlugTaken):
		writeError(c, http.StatusConflict, "SLUG_TAKEN", "短码已被占用")
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrNotStarted):
		writeError(c, http.StatusNotFound, "LINK_NOT_FOUND", "短链不存在")
	case errors.Is(err, ErrGone):
		writeError(c, http.StatusGone, "LINK_GONE", "短链已失效")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "服务器内部错误")
	}
}

// writeError 输出包含请求 ID 的统一错误响应
func writeError(c *gin.Context, status int, code, message string) {
	requestID, _ := c.Get("request_id")
	c.JSON(status, gin.H{
		"error": gin.H{
			"code":       code,
			"message":    message,
			"request_id": requestID,
		},
	})
}
