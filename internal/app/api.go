package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"

	"github.com/PYang176/Short-URL-Service/internal/analytics"
	domainpkg "github.com/PYang176/Short-URL-Service/internal/domain"
	"github.com/PYang176/Short-URL-Service/internal/identity"
	"github.com/PYang176/Short-URL-Service/internal/link"
	"github.com/PYang176/Short-URL-Service/internal/platform/cache"
	"github.com/PYang176/Short-URL-Service/internal/platform/config"
	"github.com/PYang176/Short-URL-Service/internal/platform/database"
	"github.com/PYang176/Short-URL-Service/internal/platform/idempotency"
	"github.com/PYang176/Short-URL-Service/internal/platform/observability"
	"github.com/PYang176/Short-URL-Service/internal/platform/ratelimit"
	"github.com/PYang176/Short-URL-Service/internal/transport/httpapi"
)

// API 持有 HTTP 服务及其数据库和 Redis 连接
type API struct {
	cfg    config.Config
	logger *slog.Logger
	db     *sqlx.DB
	redis  *redis.Client
	server *http.Server
}

// NewAPI 初始化必需依赖、路由和 HTTP 服务
func NewAPI(ctx context.Context, cfg config.Config, logger *slog.Logger) (*API, error) {
	db, err := database.OpenMySQL(ctx, cfg.MySQL)
	if err != nil {
		return nil, fmt.Errorf("初始化 MySQL 失败: %w", err)
	}

	redisClient, err := cache.OpenRedis(ctx, cfg.Redis)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("初始化 Redis 失败: %w", err)
	}

	registry := prometheus.NewRegistry()
	metrics := observability.NewMetrics(registry)
	health := observability.NewHealth(
		cfg.Version,
		databaseChecker{db: db},
		redisChecker{client: redisClient},
	)
	identityRepo := identity.NewMySQLRepository(db)
	passwordHasher := identity.NewPasswordHasher(identity.DefaultPasswordParams)
	tokenManager := identity.NewTokenManager(cfg.Auth.JWTSecret, cfg.Auth.AccessTokenTTL)
	identityService := identity.NewService(
		identityRepo,
		passwordHasher,
		tokenManager,
		cfg.Auth.RefreshTokenTTL,
	)
	identityHandler := identity.NewHandler(identityService)
	domainRepo := domainpkg.NewMySQLRepository(db)
	domainService := domainpkg.NewService(
		domainRepo,
		domainpkg.DNSResolver{},
		cfg.Domain.VerifyMode,
		cfg.Domain.CNAME,
	)
	domainHandler := domainpkg.NewHandler(domainService)
	linkRepo := link.NewMySQLRepository(db)
	linkService, err := link.NewService(
		linkRepo,
		redisClient,
		logger,
		cfg.PublicBaseURL,
		domainService,
	)
	if err != nil {
		return nil, fmt.Errorf("初始化短链接服务失败: %w", err)
	}
	if err := linkService.Initialize(ctx); err != nil {
		return nil, fmt.Errorf("初始化默认短链域名失败: %w", err)
	}
	clickRecorder := analytics.NewRecorder(redisClient, logger, cfg.Analytics)
	idempotencyRepo := idempotency.NewMySQLRepository(db)
	idempotencyService := idempotency.NewService(idempotencyRepo, cfg.Idempotency.TTL)
	linkHandler := link.NewHandler(linkService, clickRecorder, idempotencyService, logger)
	analyticsRepo := analytics.NewMySQLRepository(db)
	analyticsService := analytics.NewService(analyticsRepo, linkRepo)
	analyticsHandler := analytics.NewHandler(analyticsService, logger)
	slidingWindowLimiter := ratelimit.New(redisClient)

	router := httpapi.NewRouter(httpapi.Dependencies{
		Config:     cfg,
		Logger:     logger,
		Registry:   registry,
		Health:     health,
		Metrics:    metrics,
		Identity:   identityHandler,
		Links:      linkHandler,
		Domains:    domainHandler,
		Analytics:  analyticsHandler,
		Tokens:     tokenManager,
		Limiter:    slidingWindowLimiter,
		APIVersion: "v1",
	})

	server := &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           router,
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
		ReadHeaderTimeout: 5 * time.Second,
		MaxHeaderBytes:    cfg.Security.MaxHeaderBytes,
	}

	return &API{
		cfg:    cfg,
		logger: logger,
		db:     db,
		redis:  redisClient,
		server: server,
	}, nil
}

// Run 持续提供 HTTP 服务，并在上下文取消时完成优雅退出
func (a *API) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		a.logger.Info("API 服务启动", "addr", a.cfg.HTTP.Addr, "env", a.cfg.Env)
		errCh <- a.server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.HTTP.ShutdownTimeout)
		defer cancel()
		if err := a.server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("关闭 HTTP 服务失败: %w", err)
		}
		if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}

// Close 释放 API 持有的 Redis 和 MySQL 连接
func (a *API) Close() {
	if err := a.redis.Close(); err != nil {
		a.logger.Warn("关闭 Redis 失败", "error", err)
	}
	if err := a.db.Close(); err != nil {
		a.logger.Warn("关闭 MySQL 失败", "error", err)
	}
}

type databaseChecker struct {
	db *sqlx.DB
}

// Check 验证 MySQL 连接池可用
func (c databaseChecker) Check(ctx context.Context) error {
	return c.db.PingContext(ctx)
}

type redisChecker struct {
	client *redis.Client
}

// Check 验证 Redis 连接可用
func (c redisChecker) Check(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}
