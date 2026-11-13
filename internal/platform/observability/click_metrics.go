package observability

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// ClickMetrics 记录 Redis Streams 点击消费指标
type ClickMetrics struct {
	// streamLength 是当前 Stream 中的消息数量
	streamLength *prometheus.GaugeVec
	// pendingCount 是当前 Consumer Group 的 Pending 数量
	pendingCount *prometheus.GaugeVec
	// consumeTotal 是按结果记录的消费事件数量
	consumeTotal *prometheus.CounterVec
	// consumeDuration 是批次处理耗时
	consumeDuration *prometheus.HistogramVec
	// ackErrors 是 ACK 失败次数
	ackErrors *prometheus.CounterVec
}

// NewClickMetrics 在指定 Registry 注册点击消费指标
func NewClickMetrics(registry prometheus.Registerer) *ClickMetrics {
	metrics := &ClickMetrics{
		streamLength: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "shorturl",
			Subsystem: "click",
			Name:      "stream_length",
			Help:      "Current Redis Stream message count.",
		}, []string{"stream", "group"}),
		pendingCount: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "shorturl",
			Subsystem: "click",
			Name:      "pending_count",
			Help:      "Current Redis Stream pending message count.",
		}, []string{"stream", "group"}),
		consumeTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "shorturl",
			Subsystem: "click",
			Name:      "consume_total",
			Help:      "Total click events processed by result.",
		}, []string{"stream", "group", "result"}),
		consumeDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "shorturl",
			Subsystem: "click",
			Name:      "consume_duration_seconds",
			Help:      "Click event batch processing duration.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"stream", "group"}),
		ackErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "shorturl",
			Subsystem: "click",
			Name:      "ack_errors_total",
			Help:      "Total click event ACK failures.",
		}, []string{"stream", "group"}),
	}
	registry.MustRegister(
		metrics.streamLength,
		metrics.pendingCount,
		metrics.consumeTotal,
		metrics.consumeDuration,
		metrics.ackErrors,
	)
	return metrics
}

// ObserveQueue 更新 Stream 长度和 Pending 数量
func (m *ClickMetrics) ObserveQueue(stream, group string, length, pending int64) {
	if m == nil {
		return
	}
	m.streamLength.WithLabelValues(stream, group).Set(float64(length))
	m.pendingCount.WithLabelValues(stream, group).Set(float64(pending))
}

// ObserveConsume 记录一次点击批次处理结果和耗时
func (m *ClickMetrics) ObserveConsume(
	stream, group, result string,
	count int,
	duration time.Duration,
) {
	if m == nil {
		return
	}
	m.consumeTotal.WithLabelValues(stream, group, result).Add(float64(count))
	m.consumeDuration.WithLabelValues(stream, group).Observe(duration.Seconds())
}

// IncACKError 记录一次 ACK 失败
func (m *ClickMetrics) IncACKError(stream, group string) {
	if m == nil {
		return
	}
	m.ackErrors.WithLabelValues(stream, group).Inc()
}
