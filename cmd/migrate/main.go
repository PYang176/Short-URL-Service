package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/PYang176/Short-URL-Service/internal/platform/config"
	"github.com/PYang176/Short-URL-Service/internal/platform/database"
)

// main 执行数据库迁移并在失败时返回非零退出码
func main() {
	if err := run(); err != nil {
		slog.Error("数据库迁移失败", "error", err)
		os.Exit(1)
	}
}

// run 加载配置、连接 MySQL 并执行指定的 Goose 迁移命令
func run() error {
	if len(os.Args) != 2 {
		return fmt.Errorf("用法: migrate <up|down>")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := database.OpenMySQL(ctx, cfg.MySQL)
	if err != nil {
		return err
	}
	defer func() {
		if err := db.Close(); err != nil {
			slog.Warn("关闭 MySQL 失败", "error", err)
		}
	}()

	if err := goose.SetDialect("mysql"); err != nil {
		return fmt.Errorf("配置 Goose 数据库方言失败: %w", err)
	}

	runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	switch os.Args[1] {
	case "up":
		if err := goose.UpContext(runCtx, db.DB, "migrations"); err != nil {
			return fmt.Errorf("应用数据库迁移失败: %w", err)
		}
	case "down":
		if err := goose.DownContext(runCtx, db.DB, "migrations"); err != nil {
			return fmt.Errorf("回滚数据库迁移失败: %w", err)
		}
	default:
		return fmt.Errorf("未知迁移命令 %q", os.Args[1])
	}

	slog.Info("数据库迁移完成", "command", os.Args[1])
	return nil
}
