package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const (
	EnvDevelopment = "development"
	EnvTest        = "test"
	EnvProduction  = "production"
)

// Config 汇总 API 与 Worker 启动所需配置
type Config struct {
	// Env 是运行环境，有效值为 development、test、production
	Env string
	// Version 是当前构建版本
	Version string
	// PublicBaseURL 是生成默认短链地址时使用的公开根地址
	PublicBaseURL string
	// HTTP 控制 HTTP 服务超时和退出行为
	HTTP HTTPConfig
	// Auth 控制密码和 Token 生命周期
	Auth AuthConfig
	// MySQL 控制权威业务数据库连接
	MySQL MySQLConfig
	// Redis 控制缓存、限流和事件 Stream 连接
	Redis RedisConfig
	// Analytics 控制点击事件队列和访客匿名化
	Analytics AnalyticsConfig
	// Domain 控制自定义域名验证
	Domain DomainConfig
	// RateLimit 控制统一 Redis 限流策略
	RateLimit RateLimitConfig
	// Security 控制请求体和代理边界
	Security SecurityConfig
	// Idempotency 控制幂等请求保存时间
	Idempotency IdempotencyConfig
	// Trial 控制匿名游客试用会话和链接额度
	Trial TrialConfig
	// Webhook 控制 Outbox 投递轮询、超时和重试
	Webhook WebhookConfig
}

// HTTPConfig 控制 HTTP 服务端行为
type HTTPConfig struct {
	// Addr 是 API 或 Worker 健康服务监听地址
	Addr string
	// MetricsAddr 是只供内网 Prometheus 抓取的指标监听地址
	MetricsAddr string
	// ReadTimeout 是读取完整请求的最长时间
	ReadTimeout time.Duration
	// WriteTimeout 是写入完整响应的最长时间
	WriteTimeout time.Duration
	// IdleTimeout 是长连接保持空闲的最长时间
	IdleTimeout time.Duration
	// ShutdownTimeout 是优雅退出的最长等待时间
	ShutdownTimeout time.Duration
}

// AuthConfig 定义认证密钥和凭证生命周期
type AuthConfig struct {
	// JWTSecret 是 Access Token 的 HMAC 签名密钥
	JWTSecret string
	// AccessTokenTTL 是 Access Token 有效期
	AccessTokenTTL time.Duration
	// RefreshTokenTTL 是 Refresh Token 有效期
	RefreshTokenTTL time.Duration
	// PasswordResetTTL 是后续密码重置凭证有效期
	PasswordResetTTL time.Duration
}

// MySQLConfig 定义权威数据库连接与连接池参数
type MySQLConfig struct {
	// DSN 是 MySQL 连接串，包含数据库地址和凭据
	DSN string
	// MaxOpenConns 是最大打开连接数
	MaxOpenConns int
	// MaxIdleConns 是最大空闲连接数
	MaxIdleConns int
	// ConnMaxLifetime 是单条连接允许存活的最长时间
	ConnMaxLifetime time.Duration
}

// RedisConfig 定义 Redis 连接和连接池参数
type RedisConfig struct {
	// Addr 是 Redis 服务地址
	Addr string
	// Password 是 Redis 认证密码
	Password string
	// DB 是 Redis 逻辑数据库编号
	DB int
	// PoolSize 是最大连接池大小
	PoolSize int
	// MinIdleConns 是保持的最小空闲连接数
	MinIdleConns int
	// DialTimeout 是建立连接的最长时间
	DialTimeout time.Duration
	// ReadTimeout 是读取命令响应的最长时间
	ReadTimeout time.Duration
	// WriteTimeout 是写入命令的最长时间
	WriteTimeout time.Duration
}

// AnalyticsConfig 定义点击事件处理参数
type AnalyticsConfig struct {
	// HashSecret 是访客匿名哈希使用的 HMAC 密钥
	HashSecret string
	// ClickStream 是保存点击事件的 Redis Stream 名称
	ClickStream string
	// ConsumerGroup 是点击事件消费者组名称
	ConsumerGroup string
}

// DomainConfig 定义自定义域名验证参数
type DomainConfig struct {
	// VerifyMode 是验证模式，有效值为 mock 或 dns
	VerifyMode string
	// CNAME 是用户域名需要指向的平台接入域名
	CNAME string
}

// RateLimitRule 是单个接口的限流规则
type RateLimitRule struct {
	// Limit 是窗口内允许的最大请求数
	Limit int
	// Window 是滑动窗口长度
	Window time.Duration
	// FailClosed 表示 Redis 故障时拒绝请求
	FailClosed bool
}

// RateLimitConfig 汇总各接口限流规则
type RateLimitConfig struct {
	// Enabled 控制是否启用全局限流
	Enabled bool
	// Register 限制注册请求
	Register RateLimitRule
	// Login 限制登录请求
	Login RateLimitRule
	// Refresh 限制 Token 刷新
	Refresh RateLimitRule
	// API 限制普通受保护接口
	API RateLimitRule
	// CreateLink 限制创建短链
	CreateLink RateLimitRule
	// BulkCreate 限制 CSV 批量创建
	BulkCreate RateLimitRule
	// Redirect 限制公开跳转
	Redirect RateLimitRule
	// TrialCreate 限制匿名游客创建短链
	TrialCreate RateLimitRule
	// AbuseReport 限制公开滥用举报
	AbuseReport RateLimitRule
}

// SecurityConfig 定义 HTTP 请求安全边界
type SecurityConfig struct {
	// MaxBodyBytes 是 API 请求体最大字节数
	MaxBodyBytes int64
	// BulkMaxBytes 是 CSV 批量上传请求体最大字节数
	BulkMaxBytes int64
	// MaxHeaderBytes 是 HTTP Header 最大字节数
	MaxHeaderBytes int
	// TrustedProxies 是允许提供客户端 IP 的代理 CIDR
	TrustedProxies []string
}

// IdempotencyConfig 定义幂等键保存时间
type IdempotencyConfig struct {
	// TTL 是幂等结果保留时间
	TTL time.Duration
}

// TrialConfig 定义匿名游客试用限制
type TrialConfig struct {
	// SessionTTL 是游客会话和临时链接的有效期
	SessionTTL time.Duration
	// CleanupInterval 是 Worker 清理过期游客数据的周期
	CleanupInterval time.Duration
	// MaxLinks 是单个游客会话允许创建的链接数量
	MaxLinks int
}

// WebhookConfig 定义 Webhook Worker 和本地目标策略
type WebhookConfig struct {
	// PollInterval 是 Worker 查询待投递任务的周期
	PollInterval time.Duration
	// MaxAttempts 是单个投递任务允许的最大尝试次数
	MaxAttempts int
	// RequestTimeout 是单次回调 HTTP 请求的最长时间
	RequestTimeout time.Duration
	// Lease 是 Worker 锁定投递任务的最长时间
	Lease time.Duration
	// AllowPrivateTargets 允许本地演示访问 HTTP、回环和私网地址
	AllowPrivateTargets bool
}

// Load 从可选 .env 和环境变量读取配置并完成启动校验
func Load() (Config, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("加载 .env 失败: %w", err)
	}
	if err := validateEnvValueFormats(); err != nil {
		return Config{}, err
	}

	cfg := Config{
		Env:           getEnv("APP_ENV", EnvDevelopment),
		Version:       getEnv("APP_VERSION", "dev"),
		PublicBaseURL: getEnv("PUBLIC_BASE_URL", "http://localhost:8080"),
		HTTP: HTTPConfig{
			Addr:            getEnv("HTTP_ADDR", ":8080"),
			MetricsAddr:     getEnv("METRICS_ADDR", "127.0.0.1:9091"),
			ReadTimeout:     getDuration("HTTP_READ_TIMEOUT", 10*time.Second),
			WriteTimeout:    getDuration("HTTP_WRITE_TIMEOUT", 15*time.Second),
			IdleTimeout:     getDuration("HTTP_IDLE_TIMEOUT", 60*time.Second),
			ShutdownTimeout: getDuration("HTTP_SHUTDOWN_TIMEOUT", 10*time.Second),
		},
		Auth: AuthConfig{
			JWTSecret:        getEnv("JWT_SECRET", ""),
			AccessTokenTTL:   getDuration("ACCESS_TOKEN_TTL", 15*time.Minute),
			RefreshTokenTTL:  getDuration("REFRESH_TOKEN_TTL", 7*24*time.Hour),
			PasswordResetTTL: getDuration("PASSWORD_RESET_TTL", 30*time.Minute),
		},
		MySQL: MySQLConfig{
			DSN:             getEnv("MYSQL_DSN", ""),
			MaxOpenConns:    getInt("MYSQL_MAX_OPEN_CONNS", 50),
			MaxIdleConns:    getInt("MYSQL_MAX_IDLE_CONNS", 10),
			ConnMaxLifetime: getDuration("MYSQL_CONN_MAX_LIFETIME", time.Hour),
		},
		Redis: RedisConfig{
			Addr:         getEnv("REDIS_ADDR", "127.0.0.1:6379"),
			Password:     getEnv("REDIS_PASSWORD", ""),
			DB:           getInt("REDIS_DB", 0),
			PoolSize:     getInt("REDIS_POOL_SIZE", 50),
			MinIdleConns: getInt("REDIS_MIN_IDLE_CONNS", 5),
			DialTimeout:  getDuration("REDIS_DIAL_TIMEOUT", 3*time.Second),
			ReadTimeout:  getDuration("REDIS_READ_TIMEOUT", 2*time.Second),
			WriteTimeout: getDuration("REDIS_WRITE_TIMEOUT", 2*time.Second),
		},
		Analytics: AnalyticsConfig{
			HashSecret:    getEnv("ANALYTICS_HASH_SECRET", ""),
			ClickStream:   getEnv("CLICK_STREAM", "shorturl:clicks"),
			ConsumerGroup: getEnv("CLICK_CONSUMER_GROUP", "shorturl-analytics"),
		},
		Domain: DomainConfig{
			VerifyMode: getEnv("DOMAIN_VERIFY_MODE", "mock"),
			CNAME:      getEnv("CUSTOM_DOMAIN_CNAME", "links.localhost"),
		},
		RateLimit: RateLimitConfig{
			Enabled:    getBool("RATE_LIMIT_ENABLED", true),
			Register:   getRateLimitRule("RATE_LIMIT_REGISTER", 5, time.Hour, true),
			Login:      getRateLimitRule("RATE_LIMIT_LOGIN", 10, 5*time.Minute, true),
			Refresh:    getRateLimitRule("RATE_LIMIT_REFRESH", 30, time.Minute, true),
			API:        getRateLimitRule("RATE_LIMIT_API", 120, time.Minute, false),
			CreateLink: getRateLimitRule("RATE_LIMIT_CREATE_LINK", 30, time.Minute, false),
			BulkCreate: getRateLimitRule("RATE_LIMIT_BULK_CREATE", 10, time.Hour, true),
			Redirect:   getRateLimitRule("RATE_LIMIT_REDIRECT", 600, time.Minute, false),
			TrialCreate: getRateLimitRule(
				"RATE_LIMIT_TRIAL_CREATE",
				10,
				time.Hour,
				true,
			),
			AbuseReport: getRateLimitRule(
				"RATE_LIMIT_ABUSE_REPORT",
				10,
				time.Hour,
				true,
			),
		},
		Security: SecurityConfig{
			MaxBodyBytes:   int64(getInt("HTTP_MAX_BODY_BYTES", 64*1024)),
			BulkMaxBytes:   int64(getInt("HTTP_BULK_MAX_BYTES", 1024*1024)),
			MaxHeaderBytes: getInt("HTTP_MAX_HEADER_BYTES", 16*1024),
			TrustedProxies: getCSV("HTTP_TRUSTED_PROXIES", "127.0.0.1,::1"),
		},
		Idempotency: IdempotencyConfig{
			TTL: getDuration("IDEMPOTENCY_TTL", 24*time.Hour),
		},
		Trial: TrialConfig{
			SessionTTL:      getDuration("TRIAL_SESSION_TTL", 24*time.Hour),
			CleanupInterval: getDuration("TRIAL_CLEANUP_INTERVAL", 10*time.Minute),
			MaxLinks:        getInt("TRIAL_LINK_LIMIT", 3),
		},
		Webhook: WebhookConfig{
			PollInterval:        getDuration("WEBHOOK_POLL_INTERVAL", 5*time.Second),
			MaxAttempts:         getInt("WEBHOOK_MAX_ATTEMPTS", 8),
			RequestTimeout:      getDuration("WEBHOOK_REQUEST_TIMEOUT", 10*time.Second),
			Lease:               getDuration("WEBHOOK_LEASE", 30*time.Second),
			AllowPrivateTargets: getBool("WEBHOOK_ALLOW_PRIVATE_TARGETS", false),
		},
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate 拒绝会导致进程带着不安全配置启动的参数组合
func (c Config) Validate() error {
	switch c.Env {
	case EnvDevelopment, EnvTest, EnvProduction:
	default:
		return fmt.Errorf("APP_ENV 必须是 development、test 或 production")
	}

	baseURL, err := url.Parse(c.PublicBaseURL)
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		return fmt.Errorf("PUBLIC_BASE_URL 必须是绝对 URL")
	}
	if baseURL.Scheme != "http" && baseURL.Scheme != "https" {
		return fmt.Errorf("PUBLIC_BASE_URL 协议必须是 http 或 https")
	}
	if c.Env == EnvProduction && baseURL.Scheme != "https" {
		return fmt.Errorf("生产环境 PUBLIC_BASE_URL 必须使用 HTTPS")
	}
	if strings.TrimSpace(c.HTTP.Addr) == "" {
		return fmt.Errorf("HTTP_ADDR 不能为空")
	}
	if strings.TrimSpace(c.HTTP.MetricsAddr) == "" {
		return fmt.Errorf("METRICS_ADDR 不能为空")
	}
	if c.HTTP.MetricsAddr == c.HTTP.Addr {
		return fmt.Errorf("METRICS_ADDR 必须与 HTTP_ADDR 分开")
	}
	if strings.TrimSpace(c.MySQL.DSN) == "" {
		return fmt.Errorf("MYSQL_DSN 不能为空")
	}
	if strings.TrimSpace(c.Redis.Addr) == "" {
		return fmt.Errorf("REDIS_ADDR 不能为空")
	}
	if strings.TrimSpace(c.Auth.JWTSecret) == "" {
		return fmt.Errorf("JWT_SECRET 不能为空")
	}
	if strings.TrimSpace(c.Analytics.HashSecret) == "" {
		return fmt.Errorf("ANALYTICS_HASH_SECRET 不能为空")
	}
	if isPlaceholderSecret(c.Auth.JWTSecret) {
		return fmt.Errorf("JWT_SECRET 不能使用示例值")
	}
	if isPlaceholderSecret(c.Analytics.HashSecret) {
		return fmt.Errorf("ANALYTICS_HASH_SECRET 不能使用示例值")
	}
	if c.Env == EnvProduction && !isStrongSecret(c.Auth.JWTSecret) {
		return fmt.Errorf("生产环境 JWT_SECRET 至少需要 32 字节且不能使用示例值")
	}
	if c.Env == EnvProduction && !isStrongSecret(c.Analytics.HashSecret) {
		return fmt.Errorf("生产环境 ANALYTICS_HASH_SECRET 至少需要 32 字节且不能使用示例值")
	}
	if strings.TrimSpace(c.Analytics.ClickStream) == "" ||
		strings.TrimSpace(c.Analytics.ConsumerGroup) == "" {
		return fmt.Errorf("点击事件 Stream 和消费者组不能为空")
	}
	if c.Domain.VerifyMode != "mock" && c.Domain.VerifyMode != "dns" {
		return fmt.Errorf("DOMAIN_VERIFY_MODE 必须是 mock 或 dns")
	}
	if c.Env == EnvProduction && c.Domain.VerifyMode != "dns" {
		return fmt.Errorf("生产环境 DOMAIN_VERIFY_MODE 必须为 dns")
	}
	if strings.TrimSpace(c.Domain.CNAME) == "" {
		return fmt.Errorf("CUSTOM_DOMAIN_CNAME 不能为空")
	}
	if c.MySQL.MaxOpenConns <= 0 || c.MySQL.MaxIdleConns < 0 {
		return fmt.Errorf("MySQL 连接池参数无效")
	}
	if c.Redis.PoolSize <= 0 || c.Redis.MinIdleConns < 0 {
		return fmt.Errorf("redis 连接池参数无效")
	}
	if c.Security.MaxBodyBytes <= 0 || c.Security.MaxHeaderBytes <= 0 {
		return fmt.Errorf("HTTP 请求安全限制必须大于零")
	}
	if c.Security.BulkMaxBytes < c.Security.MaxBodyBytes {
		return fmt.Errorf("HTTP_BULK_MAX_BYTES 不能小于 HTTP_MAX_BODY_BYTES")
	}
	if c.Idempotency.TTL <= 0 {
		return fmt.Errorf("IDEMPOTENCY_TTL 必须大于零")
	}
	if c.Trial.SessionTTL < time.Minute || c.Trial.SessionTTL > 7*24*time.Hour {
		return fmt.Errorf("TRIAL_SESSION_TTL 必须在 1 分钟至 7 天之间")
	}
	if c.Trial.CleanupInterval < time.Minute {
		return fmt.Errorf("TRIAL_CLEANUP_INTERVAL 不能小于 1 分钟")
	}
	if c.Trial.MaxLinks < 1 || c.Trial.MaxLinks > 10 {
		return fmt.Errorf("TRIAL_LINK_LIMIT 必须在 1 至 10 之间")
	}
	if c.Webhook.PollInterval < time.Second || c.Webhook.PollInterval > time.Minute {
		return fmt.Errorf("WEBHOOK_POLL_INTERVAL 必须在 1 秒至 1 分钟之间")
	}
	if c.Webhook.MaxAttempts < 1 || c.Webhook.MaxAttempts > 20 {
		return fmt.Errorf("WEBHOOK_MAX_ATTEMPTS 必须在 1 至 20 之间")
	}
	if c.Webhook.RequestTimeout < time.Second || c.Webhook.RequestTimeout > time.Minute {
		return fmt.Errorf("WEBHOOK_REQUEST_TIMEOUT 必须在 1 秒至 1 分钟之间")
	}
	if c.Webhook.Lease <= c.Webhook.RequestTimeout || c.Webhook.Lease > 10*time.Minute {
		return fmt.Errorf("WEBHOOK_LEASE 必须大于请求超时且不超过 10 分钟")
	}
	if c.Env == EnvProduction && c.Webhook.AllowPrivateTargets {
		return fmt.Errorf("生产环境不允许 WEBHOOK_ALLOW_PRIVATE_TARGETS=true")
	}
	for name, rule := range map[string]RateLimitRule{
		"register":     c.RateLimit.Register,
		"login":        c.RateLimit.Login,
		"refresh":      c.RateLimit.Refresh,
		"api":          c.RateLimit.API,
		"create_link":  c.RateLimit.CreateLink,
		"bulk_create":  c.RateLimit.BulkCreate,
		"redirect":     c.RateLimit.Redirect,
		"trial_create": c.RateLimit.TrialCreate,
		"abuse_report": c.RateLimit.AbuseReport,
	} {
		if rule.Limit <= 0 || rule.Window <= 0 {
			return fmt.Errorf("限流规则 %s 的参数无效", name)
		}
	}
	return nil
}

// validateEnvValueFormats 在配置回退默认值前拒绝格式错误的显式环境变量。
func validateEnvValueFormats() error {
	intKeys := []string{
		"MYSQL_MAX_OPEN_CONNS",
		"MYSQL_MAX_IDLE_CONNS",
		"REDIS_DB",
		"REDIS_POOL_SIZE",
		"REDIS_MIN_IDLE_CONNS",
		"HTTP_MAX_BODY_BYTES",
		"HTTP_BULK_MAX_BYTES",
		"HTTP_MAX_HEADER_BYTES",
		"TRIAL_LINK_LIMIT",
		"WEBHOOK_MAX_ATTEMPTS",
		"RATE_LIMIT_REGISTER_LIMIT",
		"RATE_LIMIT_LOGIN_LIMIT",
		"RATE_LIMIT_REFRESH_LIMIT",
		"RATE_LIMIT_API_LIMIT",
		"RATE_LIMIT_CREATE_LINK_LIMIT",
		"RATE_LIMIT_BULK_CREATE_LIMIT",
		"RATE_LIMIT_REDIRECT_LIMIT",
		"RATE_LIMIT_TRIAL_CREATE_LIMIT",
		"RATE_LIMIT_ABUSE_REPORT_LIMIT",
	}
	for _, key := range intKeys {
		value, ok := os.LookupEnv(key)
		if !ok || strings.TrimSpace(value) == "" {
			continue
		}
		if _, err := strconv.Atoi(strings.TrimSpace(value)); err != nil {
			return fmt.Errorf("%s 必须为整数", key)
		}
	}

	durationKeys := []string{
		"HTTP_READ_TIMEOUT",
		"HTTP_WRITE_TIMEOUT",
		"HTTP_IDLE_TIMEOUT",
		"HTTP_SHUTDOWN_TIMEOUT",
		"ACCESS_TOKEN_TTL",
		"REFRESH_TOKEN_TTL",
		"PASSWORD_RESET_TTL",
		"MYSQL_CONN_MAX_LIFETIME",
		"REDIS_DIAL_TIMEOUT",
		"REDIS_READ_TIMEOUT",
		"REDIS_WRITE_TIMEOUT",
		"IDEMPOTENCY_TTL",
		"TRIAL_SESSION_TTL",
		"TRIAL_CLEANUP_INTERVAL",
		"WEBHOOK_POLL_INTERVAL",
		"WEBHOOK_REQUEST_TIMEOUT",
		"WEBHOOK_LEASE",
		"RATE_LIMIT_REGISTER_WINDOW",
		"RATE_LIMIT_LOGIN_WINDOW",
		"RATE_LIMIT_REFRESH_WINDOW",
		"RATE_LIMIT_API_WINDOW",
		"RATE_LIMIT_CREATE_LINK_WINDOW",
		"RATE_LIMIT_BULK_CREATE_WINDOW",
		"RATE_LIMIT_REDIRECT_WINDOW",
		"RATE_LIMIT_TRIAL_CREATE_WINDOW",
		"RATE_LIMIT_ABUSE_REPORT_WINDOW",
	}
	for _, key := range durationKeys {
		value, ok := os.LookupEnv(key)
		if !ok || strings.TrimSpace(value) == "" {
			continue
		}
		if _, err := time.ParseDuration(strings.TrimSpace(value)); err != nil {
			return fmt.Errorf("%s 必须为 Go Duration", key)
		}
	}

	boolKeys := []string{
		"ALLOW_MIGRATION_DOWN",
		"RATE_LIMIT_ENABLED",
		"RATE_LIMIT_REGISTER_FAIL_CLOSED",
		"RATE_LIMIT_LOGIN_FAIL_CLOSED",
		"RATE_LIMIT_REFRESH_FAIL_CLOSED",
		"RATE_LIMIT_API_FAIL_CLOSED",
		"RATE_LIMIT_CREATE_LINK_FAIL_CLOSED",
		"RATE_LIMIT_BULK_CREATE_FAIL_CLOSED",
		"RATE_LIMIT_REDIRECT_FAIL_CLOSED",
		"RATE_LIMIT_TRIAL_CREATE_FAIL_CLOSED",
		"RATE_LIMIT_ABUSE_REPORT_FAIL_CLOSED",
		"WEBHOOK_ALLOW_PRIVATE_TARGETS",
	}
	for _, key := range boolKeys {
		value, ok := os.LookupEnv(key)
		if !ok || strings.TrimSpace(value) == "" {
			continue
		}
		if _, err := strconv.ParseBool(strings.TrimSpace(value)); err != nil {
			return fmt.Errorf("%s 必须为布尔值", key)
		}
	}
	return nil
}

// getEnv 读取字符串环境变量并在缺失时返回默认值
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return strings.TrimSpace(value)
	}
	return fallback
}

// isStrongSecret 检查生产密钥长度并拒绝仓库中的示例占位值
func isStrongSecret(secret string) bool {
	value := strings.ToLower(strings.TrimSpace(secret))
	if len(value) < 32 {
		return false
	}
	return !isPlaceholderSecret(value) && !strings.HasPrefix(value, "test-")
}

// isPlaceholderSecret 检查配置模板中的固定占位值
func isPlaceholderSecret(secret string) bool {
	value := strings.ToLower(strings.TrimSpace(secret))
	return strings.HasPrefix(value, "replace-with") ||
		strings.HasPrefix(value, "change-me") ||
		strings.HasPrefix(value, "dev-only-")
}

// getInt 读取整数环境变量，格式错误时回退默认值
func getInt(key string, fallback int) int {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

// getDuration 读取 Go Duration 环境变量，格式错误时回退默认值
func getDuration(key string, fallback time.Duration) time.Duration {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

// getBool 读取布尔环境变量，格式错误时回退默认值
func getBool(key string, fallback bool) bool {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

// getCSV 读取逗号分隔列表并去除空白
func getCSV(key, fallback string) []string {
	raw := getEnv(key, fallback)
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			result = append(result, value)
		}
	}
	return result
}

// getRateLimitRule 从带前缀的环境变量读取限流规则
func getRateLimitRule(prefix string, limit int, window time.Duration, failClosed bool) RateLimitRule {
	return RateLimitRule{
		Limit:      getInt(prefix+"_LIMIT", limit),
		Window:     getDuration(prefix+"_WINDOW", window),
		FailClosed: getBool(prefix+"_FAIL_CLOSED", failClosed),
	}
}
