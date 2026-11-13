package webhook

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/PYang176/Short-URL-Service/internal/identity"
)

// Handler 提供 Webhook 配置和投递记录 HTTP 接口
type Handler struct {
	// service 管理 Webhook 生命周期
	service *Service
	// logger 记录内部错误
	logger *slog.Logger
}

// NewHandler 创建 Webhook HTTP 处理器
func NewHandler(service *Service, logger *slog.Logger) *Handler {
	return &Handler{service: service, logger: logger}
}

type createRequest struct {
	// Name 是用户可读的端点名称
	Name string `json:"name"`
	// URL 是接收事件回调的绝对地址
	URL string `json:"url"`
}

type endpointResponse struct {
	// ID 是端点 ULID
	ID string `json:"id"`
	// Name 是用户可读名称
	Name string `json:"name"`
	// URL 是回调地址
	URL string `json:"url"`
	// Status 是 1 启用或 2 停用
	Status uint8 `json:"status"`
	// FailureCount 是连续失败次数
	FailureCount int `json:"failure_count"`
	// DisabledAt 是停用 UTC 时间
	DisabledAt *time.Time `json:"disabled_at"`
	// CreatedAt 是创建 UTC 时间
	CreatedAt time.Time `json:"created_at"`
}

type createResponse struct {
	// Endpoint 是端点安全元数据
	Endpoint endpointResponse `json:"endpoint"`
	// Secret 是仅创建时返回的 HMAC 签名 Secret
	Secret string `json:"secret"`
}

type endpointListResponse struct {
	// Items 是当前工作空间端点
	Items []endpointResponse `json:"items"`
}

type deliveryResponse struct {
	// ID 是 Outbox 自增标识
	ID uint64 `json:"id"`
	// EventID 是领域事件 ULID
	EventID string `json:"event_id"`
	// EventType 是事件类型
	EventType string `json:"event_type"`
	// Status 是 1 待投递、2 投递中、3 成功或 4 终止
	Status uint8 `json:"status"`
	// AttemptCount 是已尝试次数
	AttemptCount int `json:"attempt_count"`
	// NextAttemptAt 是下一次重试 UTC 时间
	NextAttemptAt time.Time `json:"next_attempt_at"`
	// ResponseStatus 是最近 HTTP 状态码
	ResponseStatus *int64 `json:"response_status"`
	// LastError 是最近失败原因
	LastError string `json:"last_error"`
	// DeliveredAt 是成功投递 UTC 时间
	DeliveredAt *time.Time `json:"delivered_at"`
	// CreatedAt 是事件写入 Outbox 的 UTC 时间
	CreatedAt time.Time `json:"created_at"`
}

type deliveryListResponse struct {
	// Items 是按写入顺序倒序的投递记录
	Items []deliveryResponse `json:"items"`
}

// Create 创建 Webhook 端点
func (h *Handler) Create(c *gin.Context) {
	var request createRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "创建 Webhook 请求参数无效")
		return
	}
	result, err := h.service.Create(
		c.Request.Context(),
		c.GetString(identity.ContextWorkspaceID),
		request.Name,
		request.URL,
	)
	if err != nil {
		h.writeServiceError(c, err, "创建 Webhook 失败")
		return
	}
	c.JSON(http.StatusCreated, createResponse{
		Endpoint: toEndpointResponse(result.Endpoint),
		Secret:   result.Secret,
	})
}

// List 返回当前工作空间 Webhook 端点
func (h *Handler) List(c *gin.Context) {
	endpoints, err := h.service.List(
		c.Request.Context(),
		c.GetString(identity.ContextWorkspaceID),
	)
	if err != nil {
		h.logger.Error("查询 Webhook 列表失败", "error", err)
		h.writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "服务器内部错误")
		return
	}
	items := make([]endpointResponse, 0, len(endpoints))
	for index := range endpoints {
		items = append(items, toEndpointResponse(endpoints[index]))
	}
	c.JSON(http.StatusOK, endpointListResponse{Items: items})
}

// Disable 停用当前工作空间 Webhook 端点
func (h *Handler) Disable(c *gin.Context) {
	err := h.service.Disable(
		c.Request.Context(),
		c.GetString(identity.ContextWorkspaceID),
		c.Param("id"),
	)
	if err != nil {
		h.writeServiceError(c, err, "停用 Webhook 失败")
		return
	}
	c.Status(http.StatusNoContent)
}

// ListDeliveries 返回端点最近投递记录
func (h *Handler) ListDeliveries(c *gin.Context) {
	limit := 20
	if rawLimit := c.Query("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil {
			h.writeError(c, http.StatusBadRequest, "INVALID_LIMIT", "limit 必须为数字")
			return
		}
		limit = parsed
	}
	deliveries, err := h.service.ListDeliveries(
		c.Request.Context(),
		c.GetString(identity.ContextWorkspaceID),
		c.Param("id"),
		limit,
	)
	if err != nil {
		h.writeServiceError(c, err, "查询 Webhook 投递记录失败")
		return
	}
	items := make([]deliveryResponse, 0, len(deliveries))
	for index := range deliveries {
		items = append(items, toDeliveryResponse(deliveries[index]))
	}
	c.JSON(http.StatusOK, deliveryListResponse{Items: items})
}

// writeServiceError 将 Webhook 领域错误映射为 HTTP 响应
func (h *Handler) writeServiceError(c *gin.Context, err error, logMessage string) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		h.writeError(c, http.StatusBadRequest, "INVALID_WEBHOOK", err.Error())
	case errors.Is(err, ErrUnsafeTarget):
		h.writeError(c, http.StatusBadRequest, "UNSAFE_WEBHOOK_TARGET", err.Error())
	case errors.Is(err, ErrNotFound):
		h.writeError(c, http.StatusNotFound, "WEBHOOK_NOT_FOUND", "Webhook 不存在")
	default:
		h.logger.Error(logMessage, "error", err)
		h.writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "服务器内部错误")
	}
}

// writeError 输出带请求 ID 的统一错误响应
func (h *Handler) writeError(c *gin.Context, status int, code, message string) {
	requestID, _ := c.Get("request_id")
	c.JSON(status, gin.H{
		"error": gin.H{
			"code":       code,
			"message":    message,
			"request_id": requestID,
		},
	})
}

// toEndpointResponse 将端点转换为不包含密钥的 HTTP DTO
func toEndpointResponse(endpoint Endpoint) endpointResponse {
	return endpointResponse{
		ID:           endpoint.ID,
		Name:         endpoint.Name,
		URL:          endpoint.URL,
		Status:       endpoint.Status,
		FailureCount: endpoint.FailureCount,
		DisabledAt:   endpoint.DisabledAt,
		CreatedAt:    endpoint.CreatedAt,
	}
}

// toDeliveryResponse 将投递模型转换为 HTTP DTO
func toDeliveryResponse(delivery Delivery) deliveryResponse {
	var responseStatus *int64
	if delivery.ResponseStatus.Valid {
		value := delivery.ResponseStatus.Int64
		responseStatus = &value
	}
	return deliveryResponse{
		ID:             delivery.ID,
		EventID:        delivery.EventID,
		EventType:      delivery.EventType,
		Status:         delivery.Status,
		AttemptCount:   delivery.AttemptCount,
		NextAttemptAt:  delivery.NextAttemptAt,
		ResponseStatus: responseStatus,
		LastError:      delivery.LastError,
		DeliveredAt:    delivery.DeliveredAt,
		CreatedAt:      delivery.CreatedAt,
	}
}
