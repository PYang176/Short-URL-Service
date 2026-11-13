package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/PYang176/Short-URL-Service/internal/analytics"
	"github.com/PYang176/Short-URL-Service/internal/platform/cache"
	"github.com/PYang176/Short-URL-Service/internal/platform/config"
	"github.com/PYang176/Short-URL-Service/internal/platform/database"
	"github.com/PYang176/Short-URL-Service/internal/platform/observability"
	"github.com/PYang176/Short-URL-Service/internal/trial"
	"github.com/PYang176/Short-URL-Service/internal/webhook"
)

// main 启动 Worker 进程并在失败时返回非零退出码
func main() {
	if err := run(); err != nil {
		slog.Error("Worker 进程异常退出", "error", err)
		os.Exit(1)
	}
}

// run 初始化 Worker 依赖和健康服务并等待退出信号
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelStartup()

	db, err := database.OpenMySQL(startupCtx, cfg.MySQL)
	if err != nil {
		return fmt.Errorf("初始化 MySQL 失败: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			slog.Warn("关闭 MySQL 失败", "error", err)
		}
	}()

	redisClient, err := cache.OpenRedis(startupCtx, cfg.Redis)
	if err != nil {
		return fmt.Errorf("初始化 Redis 失败: %w", err)
	}
	defer func() {
		if err := redisClient.Close(); err != nil {
			slog.Warn("关闭 Redis 失败", "error", err)
		}
	}()

	registry := prometheus.NewRegistry()
	observability.RegisterRuntimeMetrics(registry)
	clickMetrics := observability.NewClickMetrics(registry)
	webhookMetrics := observability.NewWebhookMetrics(registry)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			http.Error(w, "mysql unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := redisClient.Ping(ctx).Err(); err != nil {
			http.Error(w, "redis unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.Handle("GET /metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))

	server := &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	analyticsRepo := analytics.NewMySQLRepository(db)
	clickWorker := analytics.NewWorker(
		analyticsRepo,
		redisClient,
		logger,
		cfg.Analytics.ClickStream,
		cfg.Analytics.ConsumerGroup,
		clickMetrics,
	)
	webhookCipher, err := webhook.NewSecretCipher(cfg.Auth.JWTSecret)
	if err != nil {
		return fmt.Errorf("初始化 Webhook 加密器失败: %w", err)
	}
	webhookWorker := webhook.NewWorker(
		webhook.NewMySQLRepository(db),
		webhookCipher,
		logger,
		cfg.Webhook,
		webhookMetrics,
	)

	workerCtx, stopWorkers := context.WithCancel(runCtx)
	defer stopWorkers()
	workerErrCh := make(chan error, 2)
	go func() {
		if err := clickWorker.Run(workerCtx); err != nil {
			workerErrCh <- fmt.Errorf("点击分析 Worker 异常退出: %w", err)
			return
		}
		workerErrCh <- nil
	}()
	go func() {
		if err := webhookWorker.Run(workerCtx); err != nil {
			workerErrCh <- fmt.Errorf("webhook Worker 异常退出: %w", err)
			return
		}
		workerErrCh <- nil
	}()
	trialCleaner := trial.NewCleaner(
		trial.NewMySQLRepository(db),
		logger,
		cfg.Trial.CleanupInterval,
	)
	go trialCleaner.Run(workerCtx)

	serverErrCh := make(chan error, 1)
	go func() {
		logger.Info("Worker 服务启动", "health_addr", cfg.HTTP.Addr, "env", cfg.Env)
		serverErrCh <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrCh:
		stopWorkers()
		firstWorkerErr := <-workerErrCh
		secondWorkerErr := <-workerErrCh
		if !errors.Is(err, http.ErrServerClosed) {
			return errors.Join(err, firstWorkerErr, secondWorkerErr)
		}
		return errors.Join(firstWorkerErr, secondWorkerErr)
	case err := <-workerErrCh:
		stopWorkers()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
		defer cancel()
		shutdownErr := server.Shutdown(shutdownCtx)
		otherErr := <-workerErrCh
		if !errors.Is(shutdownErr, http.ErrServerClosed) {
			return errors.Join(err, otherErr, shutdownErr)
		}
		return errors.Join(err, otherErr)
	case <-runCtx.Done():
		stopWorkers()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("关闭 Worker 健康服务失败: %w", err)
		}
		return errors.Join(<-workerErrCh, <-workerErrCh)
	}
}
