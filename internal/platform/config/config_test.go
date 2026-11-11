package config

import (
	"testing"
)

// TestLoadDefaults 验证开发和本地启动所需的最小配置可以加载
func TestLoadDefaults(t *testing.T) {
	t.Setenv("APP_ENV", EnvDevelopment)
	t.Setenv("PUBLIC_BASE_URL", "http://localhost:8080")
	t.Setenv("HTTP_ADDR", ":8080")
	t.Setenv("MYSQL_DSN", "user:pass@tcp(localhost:3306)/short_url")
	t.Setenv("REDIS_ADDR", "localhost:6379")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTP.Addr != ":8080" {
		t.Fatalf("HTTP.Addr = %q, want :8080", cfg.HTTP.Addr)
	}
	if cfg.Env != EnvDevelopment {
		t.Fatalf("Env = %q, want %q", cfg.Env, EnvDevelopment)
	}
}

// TestValidateRejectsInvalidPublicURL 验证非绝对公开地址会被拒绝
func TestValidateRejectsInvalidPublicURL(t *testing.T) {
	cfg := Config{
		Env:           EnvDevelopment,
		PublicBaseURL: "localhost:8080",
		HTTP:          HTTPConfig{Addr: ":8080"},
		MySQL:         MySQLConfig{DSN: "dsn", MaxOpenConns: 1},
		Redis:         RedisConfig{Addr: "localhost:6379", PoolSize: 1},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want invalid PUBLIC_BASE_URL error")
	}
}

// TestValidateProductionRequiresStrongJWTSecret 验证生产环境拒绝弱 JWT 密钥
func TestValidateProductionRequiresStrongJWTSecret(t *testing.T) {
	cfg := Config{
		Env:           EnvProduction,
		PublicBaseURL: "https://short.example.com",
		HTTP:          HTTPConfig{Addr: ":8080"},
		Auth:          AuthConfig{JWTSecret: "too-short"},
		MySQL:         MySQLConfig{DSN: "dsn", MaxOpenConns: 1},
		Redis:         RedisConfig{Addr: "localhost:6379", PoolSize: 1},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want JWT secret error")
	}
}
