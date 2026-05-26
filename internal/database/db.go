package database

import (
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql" // 仅导入驱动，不直接使用
	"log"
	"shortURL/internal/config"
	"time"
)

//var DB *sql.DB

// Init 初始化数据库连接
func Init(cfg *config.DBConfig) (*sql.DB, error) {
	// 连接数据库
	db, err := sql.Open("mysql", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("sql open failed: %w", err)
	}

	// 设置连接池
	// 设置最大打开连接数
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	// 设置最大空闲连接数（复用连接，提升性能）
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	// 设置连接的最大生命周期（防止连接僵死）
	db.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetimeSec) * time.Second)

	//  Ping，验证是否连接成功
	if err = db.Ping(); err != nil {
		return nil, fmt.Errorf("db ping failed: %w", err)
	}
	log.Println("Database connection established")
	return db, nil
}
