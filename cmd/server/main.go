package main

import (
	"log"
	"shortURL/internal/config"
	"shortURL/internal/database"
	"shortURL/internal/redis"
	"shortURL/internal/router"
)

func main() {
	// 加载配置
	cfg := config.Load()

	// 连接数据库
	db, err := database.Init(&cfg.DB)
	if err != nil {
		log.Fatalf("Failed to init db: %v", err)
	}
	defer db.Close()

	// 初始化Redis
	if err := redis.Init(&cfg.Redis); err != nil {
		log.Fatalf("Redis init failed: %v", err)
	}
	defer redis.RDB.Close()

	// 创建 App 并注入db
	app, err := router.NewApp(cfg, db)
	if err != nil {
		log.Fatalf("App create error: %v", err)
	}

	// 启动服务
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
