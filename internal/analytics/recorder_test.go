package analytics

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/PYang176/Short-URL-Service/internal/link"
	"github.com/PYang176/Short-URL-Service/internal/platform/config"
)

// TestRecorderWritesSanitizedEvent 验证事件写入 Stream 且不包含原始 IP
func TestRecorderWritesSanitizedEvent(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		if err := redisClient.Close(); err != nil {
			t.Errorf("关闭 Redis 失败: %v", err)
		}
	})

	recorder := NewRecorder(
		redisClient,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		config.AnalyticsConfig{
			HashSecret:  "0123456789abcdef0123456789abcdef",
			ClickStream: "clicks",
		},
	)
	recorder.now = func() time.Time {
		return time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	}

	err := recorder.RecordClick(context.Background(), link.ClickContext{
		ResolveResult: link.ResolveResult{
			LinkID:      "link-1",
			WorkspaceID: "workspace-1",
			DomainID:    "domain-1",
			Slug:        "launch",
		},
		IP:        "203.0.113.8",
		UserAgent: "Mozilla/5.0 (Windows NT 10.0) Chrome/140.0.0.0 Safari/537.36",
		Referrer:  "https://source.example/page",
		Language:  "zh-CN,zh;q=0.9",
		Country:   "cn",
		Query: url.Values{
			"utm_source":   []string{"newsletter"},
			"utm_campaign": []string{"launch"},
		},
	})
	if err != nil {
		t.Fatalf("RecordClick() error = %v", err)
	}

	messages, err := redisClient.XRange(context.Background(), "clicks", "-", "+").Result()
	if err != nil {
		t.Fatalf("XRange() error = %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("消息数量 = %d, want 1", len(messages))
	}
	raw, ok := messages[0].Values["event"].(string)
	if !ok {
		t.Fatalf("event 字段类型 = %T", messages[0].Values["event"])
	}
	if strings.Contains(raw, "203.0.113.8") {
		t.Fatal("点击事件泄露了原始 IP")
	}

	var event Event
	if err := json.Unmarshal([]byte(raw), &event); err != nil {
		t.Fatalf("解析事件失败: %v", err)
	}
	if event.Device != "desktop" || event.OS != "Windows" || event.Browser != "Chrome" {
		t.Fatalf("设备识别结果 = %s/%s/%s", event.Device, event.OS, event.Browser)
	}
	if event.Country != "CN" || event.Language != "zh-cn" {
		t.Fatalf("国家或语言 = %q/%q", event.Country, event.Language)
	}
	if event.UTMSource != "newsletter" || event.UTMCampaign != "launch" {
		t.Fatalf("UTM 解析错误: %+v", event)
	}
}

// TestDetectHelpers 验证设备、浏览器和机器人识别的关键分支
func TestDetectHelpers(t *testing.T) {
	mobile := "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) Mobile/15E148"
	if got := detectDevice(strings.ToLower(mobile)); got != "mobile" {
		t.Fatalf("detectDevice() = %q, want mobile", got)
	}
	if got := detectOS(strings.ToLower(mobile)); got != "iOS" {
		t.Fatalf("detectOS() = %q, want iOS", got)
	}
	if !isBot("Googlebot/2.1") {
		t.Fatal("Googlebot 未被识别为机器人")
	}
	if isBot("Mozilla/5.0 Chrome/140.0.0.0") {
		t.Fatal("普通 Chrome 被错误识别为机器人")
	}
}
