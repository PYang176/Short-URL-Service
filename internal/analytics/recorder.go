package analytics

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/redis/go-redis/v9"

	"github.com/PYang176/Short-URL-Service/internal/link"
	"github.com/PYang176/Short-URL-Service/internal/platform/config"
)

// Recorder 将公开跳转写入 Redis Stream
type Recorder struct {
	// redis 是点击事件 Stream 所在客户端
	redis *redis.Client
	// logger 记录非致命错误
	logger *slog.Logger
	// hashSecret 是访客匿名化 HMAC 密钥
	hashSecret []byte
	// stream 是 Redis Stream 名称
	stream string
	// now 提供可替换的 UTC 时间
	now func() time.Time
}

// NewRecorder 创建点击事件记录器
func NewRecorder(redisClient *redis.Client, logger *slog.Logger, cfg config.AnalyticsConfig) *Recorder {
	return &Recorder{
		redis:      redisClient,
		logger:     logger,
		hashSecret: []byte(cfg.HashSecret),
		stream:     cfg.ClickStream,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

// RecordClick 将点击上下文转换为事件并写入 Redis Stream
func (r *Recorder) RecordClick(ctx context.Context, click link.ClickContext) error {
	event := r.buildEvent(click, r.now())
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("序列化点击事件失败: %w", err)
	}

	result, err := r.redis.XAdd(ctx, &redis.XAddArgs{
		Stream: r.stream,
		MaxLen: 1_000_000,
		Approx: true,
		Values: map[string]any{"event": payload},
	}).Result()
	if err != nil {
		return fmt.Errorf("写入点击事件 Stream 失败: %w", err)
	}
	r.logger.Debug("点击事件已写入 Stream", "event_id", event.EventID, "stream_id", result)
	return nil
}

// buildEvent 根据请求信息生成脱敏事件
func (r *Recorder) buildEvent(click link.ClickContext, now time.Time) Event {
	userAgent := strings.ToLower(click.UserAgent)
	return Event{
		EventID:     ulid.Make().String(),
		WorkspaceID: click.WorkspaceID,
		LinkID:      click.LinkID,
		DomainID:    click.DomainID,
		Slug:        click.Slug,
		ClickedAt:   now,
		VisitorHash: visitorHash(r.hashSecret, now, click.IP, click.UserAgent),
		Country:     normalizeCountry(click.Country),
		Device:      detectDevice(userAgent),
		OS:          detectOS(userAgent),
		Browser:     detectBrowser(userAgent),
		Language:    normalizeLanguage(click.Language),
		Referrer:    truncateRunes(click.Referrer, 2048),
		UTMSource:   truncateRunes(click.Query.Get("utm_source"), 255),
		UTMMedium:   truncateRunes(click.Query.Get("utm_medium"), 255),
		UTMCampaign: truncateRunes(click.Query.Get("utm_campaign"), 255),
		UTMTerm:     truncateRunes(click.Query.Get("utm_term"), 255),
		UTMContent:  truncateRunes(click.Query.Get("utm_content"), 255),
		IsBot:       isBot(userAgent),
	}
}

// visitorHash 使用每日变化密钥生成稳定访客摘要
func visitorHash(secret []byte, now time.Time, ip, userAgent string) []byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = io.WriteString(mac, now.UTC().Format("2006-01-02"))
	_, _ = io.WriteString(mac, "|")
	_, _ = io.WriteString(mac, ip)
	_, _ = io.WriteString(mac, "|")
	_, _ = io.WriteString(mac, userAgent)
	return mac.Sum(nil)
}

// detectDevice 根据 User-Agent 粗分设备类型
func detectDevice(userAgent string) string {
	switch {
	case strings.Contains(userAgent, "ipad"), strings.Contains(userAgent, "tablet"):
		return "tablet"
	case strings.Contains(userAgent, "mobile"),
		strings.Contains(userAgent, "android"),
		strings.Contains(userAgent, "iphone"):
		return "mobile"
	default:
		return "desktop"
	}
}

// detectOS 根据 User-Agent 识别常见操作系统
func detectOS(userAgent string) string {
	switch {
	case strings.Contains(userAgent, "windows"):
		return "Windows"
	case strings.Contains(userAgent, "android"):
		return "Android"
	case strings.Contains(userAgent, "iphone"),
		strings.Contains(userAgent, "ipad"),
		strings.Contains(userAgent, "ios"):
		return "iOS"
	case strings.Contains(userAgent, "mac os"),
		strings.Contains(userAgent, "macintosh"):
		return "macOS"
	case strings.Contains(userAgent, "linux"):
		return "Linux"
	default:
		return "unknown"
	}
}

// detectBrowser 根据 User-Agent 识别常见浏览器
func detectBrowser(userAgent string) string {
	switch {
	case strings.Contains(userAgent, "edg/"):
		return "Edge"
	case strings.Contains(userAgent, "firefox/"):
		return "Firefox"
	case strings.Contains(userAgent, "chrome/"),
		strings.Contains(userAgent, "crios/"):
		return "Chrome"
	case strings.Contains(userAgent, "safari/"):
		return "Safari"
	default:
		return "unknown"
	}
}

// isBot 识别常见爬虫和自动客户端
func isBot(userAgent string) bool {
	botMarkers := []string{
		"bot", "spider", "crawler", "slurp", "headless",
		"facebookexternalhit", "bingpreview", "curl/", "wget/",
	}
	for _, marker := range botMarkers {
		if strings.Contains(userAgent, marker) {
			return true
		}
	}
	return false
}

// normalizeCountry 只接受可信代理提供的两位国家代码
func normalizeCountry(country string) string {
	country = strings.ToUpper(strings.TrimSpace(country))
	if len(country) != 2 {
		return ""
	}
	for _, character := range country {
		if character < 'A' || character > 'Z' {
			return ""
		}
	}
	return country
}

// normalizeLanguage 提取 Accept-Language 的主语言并限制长度
func normalizeLanguage(language string) string {
	language = strings.ToLower(strings.TrimSpace(strings.Split(language, ",")[0]))
	language = strings.TrimSpace(strings.Split(language, ";")[0])
	return truncateRunes(language, 16)
}

// truncateRunes 按字符截断字符串并保持合法 UTF-8
func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
