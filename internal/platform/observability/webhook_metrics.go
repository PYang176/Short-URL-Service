package observability

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// WebhookMetrics 记录 Webhook 投递结果和耗时
type WebhookMetrics struct {
	// deliveries 按事件类型和结果统计投递次数
	deliveries *prometheus.CounterVec
	// duration 记录端到端 HTTP 投递耗时
	duration *prometheus.HistogramVec
}

// NewWebhookMetrics 在指定 Registry 注册 Webhook 指标
func NewWebhookMetrics(registry prometheus.Registerer) *WebhookMetrics {
	metrics := &WebhookMetrics{
		deliveries: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "shorturl",
			Subsystem: "webhook",
			Name:      "delivery_total",
			Help:      "Total Webhook delivery attempts by result.",
		}, []string{"event_type", "result"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "shorturl",
			Subsystem: "webhook",
			Name:      "delivery_duration_seconds",
			Help:      "Webhook delivery request duration in seconds.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"event_type", "result"}),
	}
	registry.MustRegister(metrics.deliveries, metrics.duration)
	return metrics
}

// ObserveDelivery 记录一次 Webhook 投递结果
func (m *WebhookMetrics) ObserveDelivery(
	eventType, result string,
	duration time.Duration,
) {
	if m == nil {
		return
	}
	m.deliveries.WithLabelValues(eventType, result).Inc()
	m.duration.WithLabelValues(eventType, result).Observe(duration.Seconds())
}
