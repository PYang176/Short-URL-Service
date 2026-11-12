# Short URL Service

基于 Go 和 Vue 3 的短链接管理平台，覆盖用户认证、短链创建与缓存跳转、
动态二维码、自定义域名、Redis Streams 点击分析和可视化工作台。

## 已实现

- 注册、登录、Refresh Token 轮换和重放检测。
- 免登录游客试用，每个 HttpOnly Cookie 会话最多创建 3 条 24 小时短链。
- 注册后自动认领游客链接和点击数据，并清除临时过期时间。
- 用户工作空间与资源归属校验。
- 短链接创建、游标分页、详情、编辑、停用和软删除。
- 随机 Base62 短码、自定义别名和数据库唯一约束。
- URL 规范化、严格 JSON、请求体大小限制和可信代理边界。
- Redis 空值缓存、TTL 抖动、`singleflight` 和主动缓存失效。
- Redis 滑动窗口限流，覆盖注册、登录、刷新、管理接口和公开跳转。
- 创建短链 `Idempotency-Key`，相同请求可安全重放。
- Redis Streams 点击事件、Consumer Group、ACK 和 Pending 接管。
- MySQL 点击明细、趋势、独立访客、设备、系统、浏览器和来源分析。
- 动态二维码 PNG/SVG，二维码指向稳定短链并标记扫码来源。
- 自定义域名添加、TXT/CNAME 验证和 Host 跳转。
- Vue 3 管理端、项目内 SVG 趋势图和 CSS 维度排行。
- Docker Compose、Nginx、GitHub Actions 和 k6 压测报告。

## 技术栈

| 层 | 技术 |
| --- | --- |
| API | Go 1.26、Gin、sqlx |
| 数据 | MySQL 8，宿主机原生实例 |
| 缓存与事件 | Redis 7、Redis Streams |
| 前端 | Vue 3、TypeScript、Pinia、Vue Router |
| Web 服务 | Nginx |
| 可观测性 | Prometheus 指标、`log/slog` |
| 测试 | Go test、真实 MySQL 集成测试、Playwright、k6 |
| CI | GitHub Actions |

## 架构

```text
Browser
  |
Nginx :18088
  |-- Vue 静态资源
  |-- /api -> API
  `-- /r   -> API

cmd/api ----------------- MySQL（宿主机）
   |                        |
   +--- Redis cache         |
   +--- Redis rate limit    |
   +--- Redis Streams       |
             |
          cmd/worker -------+
```

API、Worker 和 Web 可以容器化。MySQL 不放入 Docker，避免本地数据库重复和
数据卷迁移；Redis 使用 Docker。

## 文档

- [需求规格](docs/需求规格.md)：当前基线与后续目标范围。
- [架构设计](docs/架构设计.md)：已实现链路和未实现模块的边界。
- [数据库设计](docs/数据库设计.md)：已落地表和目标表设计。
- [接口设计](docs/接口设计.md)：已实现接口与规划接口。
- [开发路线](docs/开发路线.md)：里程碑、验收项和当前技术债。
- [工程协作规则](AGENTS.md)：本地资源、测试和提交约束。

## 快速启动

需要：

- Go 1.26
- Node.js 24
- Docker Desktop
- 宿主机 MySQL 8

### 1. 创建 MySQL 数据库

```sql
CREATE DATABASE short_url
    CHARACTER SET utf8mb4
    COLLATE utf8mb4_unicode_ci;

CREATE USER 'short_url'@'%' IDENTIFIED BY 'change-me';
GRANT ALL PRIVILEGES ON short_url.* TO 'short_url'@'%';
FLUSH PRIVILEGES;
```

### 2. 配置环境变量

```powershell
Copy-Item deploy\.env.example deploy\.env
notepad deploy\.env
```

至少修改：

```text
MYSQL_DSN
JWT_SECRET
ANALYTICS_HASH_SECRET
```

### 3. 构建并启动

```powershell
.\scripts\start.ps1
```

脚本会先构建 Vue，再启动 Redis、迁移、API、Worker 和 Nginx。

| 服务 | 地址 |
| --- | --- |
| Web 控制台 | http://localhost:18088 |
| API | http://localhost:18080 |
| Worker 健康检查 | http://localhost:18081/health/live |
| API 就绪检查 | http://localhost:18080/health/ready |
| Prometheus 指标 | http://localhost:18080/metrics |

停止：

```powershell
docker compose --env-file deploy\.env -f deploy\docker-compose.yml down
```

## 本地开发

```powershell
Copy-Item .env.example .env
docker compose -f deploy\docker-compose.yml up -d --pull never redis
go run .\cmd\migrate up
```

终端 1：

```powershell
go run .\cmd\api
```

终端 2：

```powershell
go run .\cmd\worker
```

终端 3：

```powershell
Set-Location web
npm run dev
```

前端开发地址为 `http://127.0.0.1:5173`。本地联调时，API 的
`PUBLIC_BASE_URL` 应设置为该地址。

## 演示数据

API 启动后运行：

```powershell
.\scripts\seed-demo.ps1
```

默认账号：

```text
demo@example.com
demo12345
```

脚本会幂等创建四个演示短链，并生成未命中 UA 规则的请求和 Googlebot 请求。

## 安全与限流

滑动窗口限流保存在 Redis：

- 注册：默认 5 次/小时/IP，Redis 故障时拒绝。
- 登录：默认 10 次/5 分钟/IP，Redis 故障时拒绝。
- 刷新 Token：默认 30 次/分钟/IP，Redis 故障时拒绝。
- 管理 API：默认 120 次/分钟/工作空间。
- 创建短链：默认 30 次/分钟/工作空间。
- 公开跳转：默认 600 次/分钟/IP。

超限返回：

```http
429 Too Many Requests
Retry-After: 60
X-RateLimit-Limit: 600
X-RateLimit-Remaining: 0
```

所有策略都可通过 `.env` 覆盖。

Compose 要求显式设置 `MYSQL_DSN`、`JWT_SECRET` 和
`ANALYTICS_HASH_SECRET`，不提供内置默认密钥。缺失配置时 Compose 直接报错。
生产环境还必须设置 `APP_ENV=production`；服务会拒绝长度不足或来自示例配置
的密钥。

创建短链接支持：

```http
Idempotency-Key: create-campaign-2026-11
```

相同工作空间、相同幂等键和相同请求体返回首次创建结果，并设置：

```http
Idempotent-Replayed: true
```

免登录试用使用：

```http
POST /api/v1/trial/links
GET  /api/v1/trial/links
```

游客会话使用 HttpOnly Cookie，数据库只保存 Token 哈希。默认每 IP 每小时
允许 10 次创建请求，每个会话最多 3 条链接，链接与游客工作空间在 24 小时后
失效并由 Worker 清理。

## 主要接口

```text
POST   /api/v1/auth/register
POST   /api/v1/auth/login
POST   /api/v1/auth/refresh

POST   /api/v1/trial/links
GET    /api/v1/trial/links
POST   /api/v1/trial/claim

POST   /api/v1/links
GET    /api/v1/links
GET    /api/v1/links/{id}
PATCH  /api/v1/links/{id}
DELETE /api/v1/links/{id}
GET    /api/v1/links/{id}/qr
GET    /api/v1/links/{id}/analytics

GET    /api/v1/domains
POST   /api/v1/domains
POST   /api/v1/domains/{id}/verify
DELETE /api/v1/domains/{id}

GET /r/{slug}       默认域名
GET /{slug}         自定义域名 Host
```

默认 `GET /api/v1/links/{id}/qr` 返回 PNG，可用：

```text
?format=svg&size=512
```

## 自定义域名

本地默认：

```text
DOMAIN_VERIFY_MODE=mock
```

开发环境添加域名后可直接验证。生产环境切换为 `dns`，系统检查：

```text
_shorturl.{hostname} TXT -> verification token
{hostname} CNAME         -> CUSTOM_DOMAIN_CNAME
```

本地没有真实 DNS 时，可用 Host 请求头验证：

```powershell
curl.exe -I `
  -H "Host: go.example.com" `
  http://127.0.0.1:18080/alias
```

## 质量检查

```powershell
go test ./...
go test -race ./...
go vet ./...
golangci-lint run ./...

Set-Location web
npm run typecheck
npm run build
```

真实 MySQL 集成测试：

```powershell
$env:SHORTURL_TEST_MYSQL_DSN='user:password@tcp(127.0.0.1:3306)/short_url?parseTime=true&loc=UTC'
go test -tags=integration ./...
```

浏览器 E2E：

```powershell
python web\scripts\e2e.py
```

覆盖注册、域名验证、自定义域名短链、UA 分类、分析、二维码下载、编辑链接
和移动端布局。

## 压测结果

本机实测 500 req/s 目标，持续 20 秒：

| 指标 | 结果 |
| --- | --- |
| 跳转次数 | 10,000 |
| 实际速率 | 约 483 req/s |
| 错误率 | 0% |
| P95 | 3.55ms |
| P99 | 4.99ms |

详细环境、边界和原始 JSON：

- [500 req/s 报告](loadtest/reports/redirect-500rps.md)
- [k6 脚本](loadtest/redirect.js)

## 目录

```text
cmd/api              HTTP API 与公开跳转
cmd/worker           点击消费和后台任务
cmd/migrate          数据库迁移
internal/identity    用户、工作空间、认证
internal/link        短链、缓存、跳转和二维码
internal/domain      自定义域名
internal/analytics   点击事件和分析
internal/platform    配置、MySQL、Redis、限流、幂等
internal/transport   HTTP 路由与中间件
web                  Vue 3 管理端
loadtest             k6 脚本与实测报告
scripts              启动和演示数据脚本
migrations           MySQL 迁移
deploy               Docker Compose
```

## 当前边界

- 点击分析当前直接查询 MySQL 明细，尚未建立预聚合表。
- 机器人标记基于手写 User-Agent 子串规则，不等于验证真人。
- 浏览器当前使用 `localStorage` 保存 Access Token 和 Refresh Token；CSP
  可降低脚本注入风险，但不能替代 HttpOnly Cookie 和 CSRF 防护。
- 自定义域名生产环境尚未接 Caddy 自动证书。
- 暂未实现文件夹、标签、API Key、Webhook、管理员治理和团队角色。
- 自定义域名的管理入口仍由平台域名提供；自定义域名根路径由 API 处理。
