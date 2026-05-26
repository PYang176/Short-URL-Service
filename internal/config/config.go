package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
)

type Config struct {
	App        AppConfig        `json:"app"`
	DB         DBConfig         `json:"db"`
	Redis      RedisConfig      `json:"redis"`
	RateLimit  RateLimitConfig  `json:"rate_limit"`
	WorkerPool WorkerPoolConfig `json:"worker_pool"`
}

type AppConfig struct {
	Port string `json:"port"`
}

// DBConfig 数据库配置信息
type DBConfig struct {
	Host               string `json:"host"`
	Port               string `json:"port"`
	User               string `json:"user"`
	Password           string `json:"password"`
	Name               string `json:"name"`
	MaxOpenConns       int    `json:"max_open_conns"`
	MaxIdleConns       int    `json:"max_idle_conns"`
	ConnMaxLifetimeSec int    `json:"conn_max_lifetime_sec"`
}

// RedisConfig Redis配置信息
type RedisConfig struct {
	Addr         string `json:"addr"`
	Password     string `json:"password"`
	DB           int    `json:"db"`
	PoolSize     int    `json:"pool_size"`
	MinIdleConns int    `json:"min_idle_conns"`
}

// RateLimitConfig 限流配置结构
type RateLimitConfig struct {
	Enabled bool                        `json:"enabled"`
	Default RouteLimitConfig            `json:"default"`
	Routes  map[string]RouteLimitConfig `json:"routes"`
}

type RouteLimitConfig struct {
	Limit     int `json:"limit"`      // 窗口内最大请求数
	WindowSec int `json:"window_sec"` // 时间窗口（秒）
}

// WorkerPoolConfig 协程池配置结构
type WorkerPoolConfig struct {
	WorkerNum int `json:"worker_num"` // 工作协程数量
	QueueSize int `json:"queue_size"` // 任务队列大小
}

// DSN 返回数据库连接字符串
func (c *DBConfig) DSN() string {
	return fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		c.User,
		c.Password,
		c.Host,
		c.Port,
		c.Name,
	)
}

// Load 从 config.json 文件中加载配置
func Load() *Config {
	// 1. 打开文件
	file, err := os.Open("config.json")
	if err != nil {
		log.Fatalf("Failed to open config file: %v", err)
	}
	defer file.Close()

	// 2. 解码 JSON
	var cfg Config
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&cfg); err != nil {
		log.Fatalf("Failed to decode config JSON: %v", err)
	}

	log.Println("Config loaded successfully from config.json")
	return &cfg
}
