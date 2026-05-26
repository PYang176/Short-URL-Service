# 短链服务 (Short URL Service)

基于 Go 1.22 原生 `net/http` 开发的高性能、高可Short URL Service用短链跳转系统。

## 功能特性

- **高性能路由**：基于 Go 1.22 原生 `net/http`，无第三方框架依赖。
- **Redis 缓存**：缓存热点数据，结合空值缓存有效防止缓存穿透。
- **分布式限流**：Redis 固定窗口计数 + Lua 原子操作，防止恶意刷接口。
- **协程池**：异步处理点击量更新，控制 Goroutine 并发数，削峰填谷。
- **配置化**：JSON 统一管理 DB、Redis、限流及协程池参数，支持环境隔离。

## 快速开始

### 环境准备

- Go 1.22+
- MySQL 8.0+
- Redis 7.0+

### 配置数据库

### 1. 配置 Go 镜像源

- go env -w GO111MODULE=on
- go env -w GOPROXY=https://goproxy.cn,direct

### 2. 安装依赖

- go get -u github.com/go-sql-driver/mysql
- go get github.com/redis/go-redis/v9

### 3. 初始化数据库(`init.sql`)

### 4. 配置文件 (`config.json`)

### 4. 启动服务

bash

go mod tidy

go run cmd/server/main.go

## API 文档

### 创建短链

bash

curl -X POST http://localhost:8080/create
\

-H "Content-Type: application/json" \

-d '{"original_url": "https://www.baidu.com
"}'

**响应示例：**
json {

"short_code": "aZ9x2P",

"full_url": "http://localhost:8080/r/aZ9x2P",

"message": "Short URL created successfully"

}

### 访问短链
http://localhost:8080/r/aZ9x2P

##  架构图
Client → 限流中间件 → Go Service → Redis → MySQL

↓

协程池（异步更新点击量）

## 📖 技术选型说明
- **为什么不用 Gin？** 学习 Go 原生 HTTP 原理
- **为什么用 Lua 脚本？** 保证 `INCR` 和 `EXPIRE` 的原子性，防止并发下限流失效。