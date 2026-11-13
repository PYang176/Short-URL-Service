package observability

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics 汇总 HTTP 请求计数和延迟指标
type Metrics struct {
	// requests 按方法、路由和状态码统计请求数
	requests *prometheus.CounterVec
	// duration 按方法、路由和状态码记录延迟
	duration *prometheus.HistogramVec
}

// NewMetrics 在指定 Registry 注册应用指标
func NewMetrics(registry prometheus.Registerer) *Metrics {
	RegisterRuntimeMetrics(registry)
	m := &Metrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "shorturl",
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "Total number of HTTP requests.",
		}, []string{"method", "route", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "shorturl",
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "HTTP request latency in seconds.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"method", "route", "status"}),
	}

	registry.MustRegister(m.requests, m.duration)
	return m
}

// Middleware 记录 Gin 请求状态和耗时
func (m *Metrics) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		status := strconv.Itoa(c.Writer.Status())
		labels := []string{c.Request.Method, route, status}
		m.requests.WithLabelValues(labels...).Inc()
		m.duration.WithLabelValues(labels...).Observe(time.Since(started).Seconds())
	}
}
