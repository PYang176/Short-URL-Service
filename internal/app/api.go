package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"

	"github.com/PYang176/Short-URL-Service/internal/abuse"
	"github.com/PYang176/Short-URL-Service/internal/analytics"
	"github.com/PYang176/Short-URL-Service/internal/apikey"
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
	trialpkg "github.com/PYang176/Short-URL-Service/internal/trial"
	"github.com/PYang176/Short-URL-Service/internal/webhook"
)

// API 持有 HTTP 服务及其数据库和 Redis 连接
type API struct {
	cfg    config.Config
	logger *slog.Logger
	db     *sqlx.DB
	redis  *redis.Client
	server *http.Server
	// metricsServer 只在内部指标端口提供 Prometheus 抓取
	metricsServer *http.Server
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
	abuseRepo := abuse.NewMySQLRepository(db)
	linkService, err := link.NewService(
		linkRepo,
		redisClient,
		logger,
		cfg.PublicBaseURL,
		domainService,
		abuse.NewDestinationChecker(abuseRepo),
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
	abuseService := abuse.NewService(abuseRepo, linkService)
	abuseHandler := abuse.NewHandler(abuseService, logger)
	analyticsRepo := analytics.NewMySQLRepository(db)
	analyticsService := analytics.NewService(analyticsRepo, linkRepo)
	analyticsHandler := analytics.NewHandler(analyticsService, logger)
	slidingWindowLimiter := ratelimit.New(redisClient)
	apiKeyRepo := apikey.NewMySQLRepository(db)
	apiKeyService := apikey.NewService(apiKeyRepo)
	apiKeyHandler := apikey.NewHandler(apiKeyService, logger)
	trialRepo := trialpkg.NewMySQLRepository(db)
	trialService := trialpkg.NewService(
		trialRepo,
		cfg.Trial.SessionTTL,
		cfg.Trial.MaxLinks,
	)
	trialHandler := trialpkg.NewHandler(trialService, linkService, logger)
	webhookCipher, err := webhook.NewSecretCipher(cfg.Auth.JWTSecret)
	if err != nil {
		return nil, fmt.Errorf("初始化 Webhook 加密器失败: %w", err)
	}
	webhookRepo := webhook.NewMySQLRepository(db)
	webhookService := webhook.NewService(
		webhookRepo,
		webhookCipher,
		cfg.Webhook.AllowPrivateTargets,
	)
	webhookHandler := webhook.NewHandler(webhookService, logger)

	router := httpapi.NewRouter(httpapi.Dependencies{
		Config:     cfg,
		Logger:     logger,
		Health:     health,
		Metrics:    metrics,
		Identity:   identityHandler,
		Links:      linkHandler,
		Domains:    domainHandler,
		Analytics:  analyticsHandler,
		APIKeys:    apiKeyHandler,
		APIKeyAuth: apiKeyService,
		Trial:      trialHandler,
		Webhooks:   webhookHandler,
		Abuse:      abuseHandler,
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
	metricsMux := http.NewServeMux()
	metricsMux.Handle("GET /metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	metricsServer := &http.Server{
		Addr:              cfg.HTTP.MetricsAddr,
		Handler:           metricsMux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return &API{
		cfg:           cfg,
		logger:        logger,
		db:            db,
		redis:         redisClient,
		server:        server,
		metricsServer: metricsServer,
	}, nil
}

// Run 持续提供 HTTP 服务，并在上下文取消时完成优雅退出
func (a *API) Run(ctx context.Context) error {
	type serverResult struct {
		name string
		err  error
	}
	errCh := make(chan serverResult, 2)
	go func() {
		a.logger.Info("API 服务启动", "addr", a.cfg.HTTP.Addr, "env", a.cfg.Env)
		errCh <- serverResult{name: "API", err: a.server.ListenAndServe()}
	}()
	go func() {
		a.logger.Info("API 指标服务启动", "addr", a.cfg.HTTP.MetricsAddr)
		errCh <- serverResult{name: "指标", err: a.metricsServer.ListenAndServe()}
	}()

	select {
	case first := <-errCh:
		shutdownErr := shutdownServers(a.cfg.HTTP.ShutdownTimeout, a.server, a.metricsServer)
		second := <-errCh
		return errors.Join(
			serverError(first.name, first.err),
			serverError(second.name, second.err),
			shutdownErr,
		)
	case <-ctx.Done():
		shutdownErr := shutdownServers(a.cfg.HTTP.ShutdownTimeout, a.server, a.metricsServer)
		first := <-errCh
		second := <-errCh
		return errors.Join(
			serverError(first.name, first.err),
			serverError(second.name, second.err),
			shutdownErr,
		)
	}
}

// shutdownServers 在共享超时内并行关闭业务和指标 HTTP 服务
func shutdownServers(timeout time.Duration, servers ...*http.Server) error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	errCh := make(chan error, len(servers))
	var wait sync.WaitGroup
	wait.Add(len(servers))
	for _, server := range servers {
		go func(server *http.Server) {
			defer wait.Done()
			if err := server.Shutdown(shutdownCtx); err != nil {
				errCh <- fmt.Errorf("关闭 HTTP 服务失败: %w", err)
			}
		}(server)
	}
	wait.Wait()
	close(errCh)

	errs := make([]error, 0)
	for err := range errCh {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// serverError 忽略正常的 HTTP 关闭错误并保留监听失败原因
func serverError(name string, err error) error {
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return fmt.Errorf("%s HTTP 服务异常退出: %w", name, err)
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
