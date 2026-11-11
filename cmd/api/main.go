package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/PYang176/Short-URL-Service/internal/app"
	"github.com/PYang176/Short-URL-Service/internal/platform/config"
)

// main 启动 API 进程并在失败时返回非零退出码
func main() {
	if err := run(); err != nil {
		slog.Error("API 进程异常退出", "error", err)
		os.Exit(1)
	}
}

// run 加载配置、初始化日志和 API，并等待退出信号
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel(cfg.Env),
	}))
	slog.SetDefault(logger)

	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelStartup()

	api, err := app.NewAPI(startupCtx, cfg, logger)
	if err != nil {
		return err
	}
	defer api.Close()

	runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return api.Run(runCtx)
}

// logLevel 根据运行环境选择结构化日志级别
func logLevel(env string) slog.Level {
	if env == config.EnvProduction {
		return slog.LevelInfo
	}
	return slog.LevelDebug
}
