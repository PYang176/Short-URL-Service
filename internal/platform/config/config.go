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
}

// HTTPConfig 控制 HTTP 服务端行为
type HTTPConfig struct {
	// Addr 是 API 或 Worker 健康服务监听地址
	Addr string
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

// Load 从可选 .env 和环境变量读取配置并完成启动校验
func Load() (Config, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("加载 .env 失败: %w", err)
	}

	cfg := Config{
		Env:           getEnv("APP_ENV", EnvDevelopment),
		Version:       getEnv("APP_VERSION", "dev"),
		PublicBaseURL: getEnv("PUBLIC_BASE_URL", "http://localhost:8080"),
		HTTP: HTTPConfig{
			Addr:            getEnv("HTTP_ADDR", ":8080"),
			ReadTimeout:     getDuration("HTTP_READ_TIMEOUT", 10*time.Second),
			WriteTimeout:    getDuration("HTTP_WRITE_TIMEOUT", 15*time.Second),
			IdleTimeout:     getDuration("HTTP_IDLE_TIMEOUT", 60*time.Second),
			ShutdownTimeout: getDuration("HTTP_SHUTDOWN_TIMEOUT", 10*time.Second),
		},
		Auth: AuthConfig{
			JWTSecret:        getEnv("JWT_SECRET", "dev-only-secret-change-before-production-123456"),
			AccessTokenTTL:   getDuration("ACCESS_TOKEN_TTL", 15*time.Minute),
			RefreshTokenTTL:  getDuration("REFRESH_TOKEN_TTL", 7*24*time.Hour),
			PasswordResetTTL: getDuration("PASSWORD_RESET_TTL", 30*time.Minute),
		},
		MySQL: MySQLConfig{
			DSN: getEnv(
				"MYSQL_DSN",
				"root:password@tcp(127.0.0.1:3306)/short_url?parseTime=true&loc=UTC&charset=utf8mb4&collation=utf8mb4_unicode_ci",
			),
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
			HashSecret:    getEnv("ANALYTICS_HASH_SECRET", "dev-only-analytics-secret-change-in-production"),
			ClickStream:   getEnv("CLICK_STREAM", "shorturl:clicks"),
			ConsumerGroup: getEnv("CLICK_CONSUMER_GROUP", "shorturl-analytics"),
		},
		Domain: DomainConfig{
			VerifyMode: getEnv("DOMAIN_VERIFY_MODE", "mock"),
			CNAME:      getEnv("CUSTOM_DOMAIN_CNAME", "links.localhost"),
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
	if strings.TrimSpace(c.HTTP.Addr) == "" {
		return fmt.Errorf("HTTP_ADDR 不能为空")
	}
	if strings.TrimSpace(c.MySQL.DSN) == "" {
		return fmt.Errorf("MYSQL_DSN 不能为空")
	}
	if strings.TrimSpace(c.Redis.Addr) == "" {
		return fmt.Errorf("REDIS_ADDR 不能为空")
	}
	if c.Env == EnvProduction && len(c.Auth.JWTSecret) < 32 {
		return fmt.Errorf("生产环境 JWT_SECRET 至少需要 32 字节")
	}
	if c.Env == EnvProduction && len(c.Analytics.HashSecret) < 32 {
		return fmt.Errorf("生产环境 ANALYTICS_HASH_SECRET 至少需要 32 字节")
	}
	if strings.TrimSpace(c.Analytics.ClickStream) == "" ||
		strings.TrimSpace(c.Analytics.ConsumerGroup) == "" {
		return fmt.Errorf("点击事件 Stream 和消费者组不能为空")
	}
	if c.Domain.VerifyMode != "mock" && c.Domain.VerifyMode != "dns" {
		return fmt.Errorf("DOMAIN_VERIFY_MODE 必须是 mock 或 dns")
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
	return nil
}

// getEnv 读取字符串环境变量并在缺失时返回默认值
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return strings.TrimSpace(value)
	}
	return fallback
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
