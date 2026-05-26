package router

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"shortURL/internal/middleware"
	"shortURL/internal/pool"
	"time"

	"shortURL/internal/config"
	"shortURL/internal/handler"
	"shortURL/internal/repository"
	"shortURL/internal/service"
)

// App 是整个应用的容器
type App struct {
	cfg      *config.Config
	db       *sql.DB
	server   *http.Server
	taskPool *pool.Pool
}

// NewApp 创建并初始化应用
func NewApp(cfg *config.Config, db *sql.DB) (*App, error) {

	// 创建协程池
	taskPool := pool.NewPool(cfg.WorkerPool.WorkerNum, cfg.WorkerPool.QueueSize)
	taskPool.Start()

	urlRepo := repository.NewURLRepo(db)
	urlSvc := service.NewURLService(urlRepo, taskPool)
	urlH := handler.NewURLHandler(urlSvc)

	// 创建自定义 ServeMux
	mux := http.NewServeMux()

	// 注册路由
	mux.HandleFunc("/health", handler.HealthCheck(db))

	// 对创建接口进行限流
	createLimiter := middleware.RedisRateLimit(cfg.RateLimit.Routes["/create"])
	mux.Handle("/create", createLimiter(http.HandlerFunc(urlH.Create)))

	// 对重定向接口进行限流
	redirectLimiter := middleware.RedisRateLimit(cfg.RateLimit.Routes["/r/{code}"])
	mux.Handle("/r/{code}", redirectLimiter(http.HandlerFunc(urlH.Redirect)))

	// 用中间件包裹
	finalHandler := middleware.Recovery(middleware.Logging(mux))

	// 创建 HTTP Server
	server := &http.Server{
		Addr:         cfg.App.Port,
		Handler:      finalHandler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return &App{
		cfg:      cfg,
		db:       db,
		taskPool: taskPool,
		server:   server,
	}, nil
}

// Run 启动 HTTP 服务
func (a *App) Run() error {
	log.Printf("Server starting on %s", a.cfg.App.Port)
	return a.server.ListenAndServe()
}

// Shutdown 关机
func (a *App) Shutdown(ctx context.Context) error {
	// 先停止接收新请求
	if err := a.server.Shutdown(ctx); err != nil {
		return err
	}
	// 停止协程池，等待任务处理完
	a.taskPool.Stop()
	// 关闭 DB
	if a.db != nil {
		a.db.Close()
	}
	return nil
}
