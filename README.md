# Short URL Service

一个面向高并发跳转和访问分析的短链接平台。项目采用模块化单体结构，提供 `api` 和 `worker` 两个独立运行角色，覆盖短链管理、自定义域名、动态二维码、点击分析和可视化报表。

> 当前处于完整版开发阶段。认证、工作空间、数据库迁移和本地运行环境已经完成，短链、域名、二维码和分析模块正在按路线逐步实现。

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
- Docker Compose 完整本地依赖。

## 技术栈

| 层 | 技术 |
| --- | --- |
| API | Go、Gin、sqlx |
| 事务数据库 | MySQL 8 |
| 缓存与事件 | Redis 7、Redis Streams |
| 分析数据库 | ClickHouse |
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
             |
          ClickHouse
```

详细设计见：

- [需求规格](docs/需求规格.md)
- [架构设计](docs/架构设计.md)
- [数据库设计](docs/数据库设计.md)
- [开发路线](docs/开发路线.md)
- [接口设计](docs/接口设计.md)

## 快速启动

需要 Docker Desktop。

```powershell
Copy-Item deploy\.env.example deploy\.env
docker compose --env-file deploy\.env -f deploy\docker-compose.yml up -d --build
```

默认地址：

| 服务 | 地址 |
| --- | --- |
| API | http://localhost:18080 |
| Worker 健康检查 | http://localhost:18081/health/live |
| 存活检查 | http://localhost:18080/health/live |
| 就绪检查 | http://localhost:18080/health/ready |
| Prometheus 指标 | http://localhost:18080/metrics |

MySQL 和 Redis 默认不映射到宿主机端口，避免与本地已有实例冲突。需要直接访问时可通过容器内网或临时 Compose override。

停止环境：

```powershell
docker compose --env-file deploy\.env -f deploy\docker-compose.yml down
```

## 本地开发

先启动 MySQL 和 Redis，然后执行迁移：

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

## 质量检查

```powershell
go test ./...
go test -race ./...
go vet ./...
golangci-lint run ./...
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
- ClickHouse 分析链路将在点击事件模块中接入。
- 旧的实验实现已移除，历史代码仍可从初始 Git 提交恢复。
