package observability

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// TestClickMetrics 验证 Stream 队列、消费和 ACK 指标写入
func TestClickMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewClickMetrics(registry)

	metrics.ObserveQueue("clicks", "analytics", 12, 3)
	if got := metricValue(t, registry, "shorturl_click_stream_length"); got != 12 {
		t.Fatalf("stream length = %v, want 12", got)
	}
	if got := metricValue(t, registry, "shorturl_click_pending_count"); got != 3 {
		t.Fatalf("pending count = %v, want 3", got)
	}

	metrics.ObserveConsume("clicks", "analytics", "success", 5, 20*time.Millisecond)
	if got := metricValue(t, registry, "shorturl_click_consume_total"); got != 5 {
		t.Fatalf("consume total = %v, want 5", got)
	}

	metrics.IncACKError("clicks", "analytics")
	if got := metricValue(t, registry, "shorturl_click_ack_errors_total"); got != 1 {
		t.Fatalf("ACK errors = %v, want 1", got)
	}
}

// metricValue 从 Registry 快照中读取单序列 gauge 或 counter 值
func metricValue(t *testing.T, registry *prometheus.Registry, name string) float64 {
	t.Helper()
	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	for _, family := range families {
		if family.GetName() != name || len(family.Metric) == 0 {
			continue
		}
		metric := family.Metric[0]
		if metric.Gauge != nil {
			return metric.Gauge.GetValue()
		}
		if metric.Counter != nil {
			return metric.Counter.GetValue()
		}
	}
	t.Fatalf("metric %q not found", name)
	return 0
}
