# Short URL Service

一个面向高并发跳转和访问分析的短链接平台。项目采用模块化单体结构，提供 `api` 和 `worker` 两个独立运行角色，覆盖短链管理、自定义域名、动态二维码、点击分析和可视化报表。

> 当前处于完整版开发阶段。认证、工作空间、短链 CRUD、默认域名跳转和 Redis 缓存已经完成，二维码、自定义域名和点击分析正在按路线逐步实现。

## 已实现

- Gin API 和 Worker 双入口。
- 环境变量配置和启动校验。
- 结构化 JSON 日志、请求 ID 和 Prometheus 指标。
- MySQL、Redis 连接池。
- 存活检查、就绪检查和优雅退出。
- Goose 数据库迁移。
- 用户注册、登录、退出。
- Argon2id 密码摘要。
- JWT Access Token。
- 不透明 Refresh Token 和轮换重放检测。
- 注册时原子创建用户、工作空间和 Owner 成员关系。
- 当前用户资料查询和修改。
- 短链接创建、列表、详情、修改、停用和软删除。
- 随机 Base62 短码、自定义别名和短码唯一约束。
- 目标 URL 规范化和安全校验。
- Redis 跳转缓存、空值缓存、TTL 抖动和并发回源合并。
- 修改或删除链接后的缓存主动失效。
- `GET /r/{slug}` 默认域名公开跳转。
- Docker Compose 管理 Redis、API、Worker 和迁移任务。

## 技术栈

| 层 | 技术 |
| --- | --- |
| API | Go、Gin、sqlx |
| 事务数据库 | MySQL 8 |
| 缓存与事件 | Redis 7、Redis Streams |
| 分析存储 | MySQL 8 点击明细与预聚合表 |
| 前端 | Vue 3、TypeScript、ECharts |
| 对象存储 | MinIO |
| 反向代理 | Caddy |
| 可观测性 | Prometheus、Grafana、`log/slog` |

## 架构

```text
Vue Web
   |
Caddy
   |
cmd/api ---------------------- MySQL
   |                             |
   +---- Redis cache/rate limit  |
   |                             |
   +---- Redis Streams           |
             |                   |
          cmd/worker ------------+
```

详细设计见：

- [需求规格](docs/需求规格.md)
- [架构设计](docs/架构设计.md)
- [数据库设计](docs/数据库设计.md)
- [数据库查询与索引设计](docs/数据库查询设计.md)
- [开发路线](docs/开发路线.md)
- [接口设计](docs/接口设计.md)

## 快速启动

需要 Docker Desktop，以及宿主机本地 MySQL 8。

先在本地 MySQL 创建数据库和专用账号：

```sql
CREATE DATABASE short_url
    CHARACTER SET utf8mb4
    COLLATE utf8mb4_unicode_ci;

CREATE USER 'short_url'@'%' IDENTIFIED BY 'change-me';
GRANT ALL PRIVILEGES ON short_url.* TO 'short_url'@'%';
FLUSH PRIVILEGES;
```

账号 Host 使用 `%` 是为了允许 Docker Desktop 容器通过
`host.docker.internal` 访问宿主机 MySQL。实际使用时请设置强密码，并保证
MySQL 只监听可信网络。

```powershell
Copy-Item deploy\.env.example deploy\.env
notepad deploy\.env
docker compose --env-file deploy\.env -f deploy\docker-compose.yml up -d --build
```

`deploy\.env` 中的 `MYSQL_DSN` 必须指向自己的本地 MySQL。容器模式下默认使用：

```text
host.docker.internal:3306
```

默认地址：

| 服务 | 地址 |
| --- | --- |
| API | http://localhost:18080 |
| Worker 健康检查 | http://localhost:18081/health/live |
| 存活检查 | http://localhost:18080/health/live |
| 就绪检查 | http://localhost:18080/health/ready |
| Prometheus 指标 | http://localhost:18080/metrics |

MySQL 不放入 Docker，也不由 Compose 创建数据卷或端口映射。Redis、API 和
Worker 在 Compose 中运行，迁移任务直接连接宿主机 MySQL。

停止环境：

```powershell
docker compose --env-file deploy\.env -f deploy\docker-compose.yml down
```

## 本地开发

先启动宿主机 MySQL 和 Redis，然后执行迁移：

```powershell
Copy-Item .env.example .env
go run .\cmd\migrate up
go run .\cmd\api
```

另开终端启动 Worker：

```powershell
go run .\cmd\worker
```

本地 API 默认监听 `:8080`。如果与已有服务冲突，修改 `.env` 中的 `HTTP_ADDR`。

## 认证接口

### 注册

```http
POST /api/v1/auth/register
Content-Type: application/json

{
  "email": "user@example.com",
  "password": "password123",
  "display_name": "User"
}
```

注册成功后返回用户、默认工作空间、Access Token 和 Refresh Token。

### 登录

```http
POST /api/v1/auth/login
Content-Type: application/json

{
  "email": "user@example.com",
  "password": "password123"
}
```

### 刷新令牌

```http
POST /api/v1/auth/refresh
Content-Type: application/json

{
  "refresh_token": "srt_..."
}
```

Refresh Token 每次使用后都会轮换。旧 Token 再次使用会被判定为泄漏并撤销整个 Token Family。

### 当前用户

```http
GET /api/v1/me
Authorization: Bearer <access-token>
```

### 创建短链

```http
POST /api/v1/links
Authorization: Bearer <access-token>
Content-Type: application/json

{
  "destination_url": "https://example.com/landing",
  "alias": "launch",
  "title": "Launch page",
  "redirect_code": 302
}
```

创建成功后返回完整短链，例如：

```text
http://localhost:18080/r/launch
```

### 链接管理

```text
GET    /api/v1/links
GET    /api/v1/links/{id}
PATCH  /api/v1/links/{id}
DELETE /api/v1/links/{id}
```

### 公开跳转

```text
GET /r/{slug}
```

## 质量检查

```powershell
go test ./...
go test -race ./...
go vet ./...
golangci-lint run ./...
```

真实 MySQL 集成测试：

```powershell
$env:SHORTURL_TEST_MYSQL_DSN='user:password@tcp(127.0.0.1:3306)/short_url?parseTime=true&loc=UTC'
go test -tags=integration ./internal/link
```

## 目录

```text
cmd/
  api/          HTTP API 和公开跳转入口
  worker/       点击消费、域名验证和后台任务入口
  migrate/      数据库迁移入口
internal/
  identity/     用户、工作空间和认证
  platform/     配置、MySQL、Redis、可观测性
  transport/    HTTP 中间件与路由
  app/          应用装配和生命周期
migrations/     MySQL 版本化迁移
deploy/         Docker Compose 和部署配置
docs/           需求、架构、数据和开发文档
```

## 当前边界

- 自定义域名将使用 DNS Resolver 抽象，在没有真实域名时通过 Fake Resolver 和 Host 头完成本地验证。
- 点击分析使用 Redis Streams 异步缓冲，并通过 MySQL 明细表和每日聚合表提供统计查询。
- 当前只启用平台默认域名，自定义域名和二维码尚未接入。
- 旧的实验实现已移除，历史代码仍可从初始 Git 提交恢复。
