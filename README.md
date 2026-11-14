# Short URL Service

基于 Go 和 Vue 3 的短链接管理平台，覆盖用户认证、短链创建与缓存跳转、
动态二维码、自定义域名、Redis Streams 点击分析和可视化工作台。短链变更
通过 MySQL 事务型 Outbox 投递为带 HMAC 签名的 Webhook。

普通用户界面只保留短链创建、二维码、跳转和访问分析。域名、API Key、
Webhook 等后台配置只对平台管理员开放，Prometheus 指标使用独立内网端口。

## 已实现

- 注册、登录、Refresh Token 轮换和重放检测。
- 免登录游客试用，每条链接可选择 15 分钟、30 分钟、1 小时、12 小时或
  24 小时有效期；会话最多创建 3 条链接。
- 注册后自动认领游客链接和点击数据，并清除临时过期时间。
- 未登录访问根路径直接进入游客工作空间；已登录用户进入 Dashboard。
- 用户工作空间与资源归属校验。
- 短链接创建、游标分页、详情、编辑、停用和软删除。
- 随机 Base62 短码、自定义别名和数据库唯一约束。
- URL 规范化、严格 JSON、请求体大小限制和可信代理边界。
- Redis 空值缓存、TTL 抖动、`singleflight` 和主动缓存失效。
- Redis 滑动窗口限流，覆盖注册、登录、刷新、管理接口和公开跳转。
- 创建短链 `Idempotency-Key`，相同请求可安全重放。
- Redis Streams 点击事件、Consumer Group、ACK 和 Pending 接管。
- MySQL 点击明细、趋势、独立访客、设备、系统、浏览器和来源分析。
- 工作空间总链接、启用链接、区间点击、独立访客、整体趋势和 Top 短链。
- 点击明细支持时间、链接、真人/机器人、设备、国家、浏览器和 UTM 组合筛选，
  并通过 MySQL 游标流式导出 CSV。
- CSV 同步批量创建，单文件最多 500 行，逐行返回成功、冲突和校验错误。
- API Key 生命周期、作用域认证、撤销、按 Key 限流和 OpenAPI 3.1 文档。
- Webhook 端点管理、事务型 Outbox、HMAC-SHA256 签名、重试和自动停用。
- 公开滥用举报、管理员下架、目标域名黑名单和后台操作审计。
- 动态二维码 PNG/SVG，二维码指向稳定短链并标记扫码来源。
- 自定义域名添加、TXT/CNAME 验证和 Host 跳转。
- Vue 3 管理端、项目内 SVG 趋势图和 CSS 维度排行。
- Docker Compose、Nginx、GitHub Actions 和 k6 压测报告。

## 技术栈

| 层 | 技术 |
| --- | --- |
| API | Go 1.26.11、Gin、sqlx |
| 数据 | MySQL 8，宿主机原生实例 |
| 缓存与事件 | Redis 7、Redis Streams |
| 前端 | Vue 3、TypeScript、Pinia、Vue Router |
| Web 服务 | Nginx |
| 可观测性 | Prometheus、Grafana、`log/slog` |
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

API、Worker 和迁移任务共用单一 `shorturl-app:local` 镜像，只是执行命令不同；
Web 使用独立的 Nginx 镜像。MySQL 不放入 Docker，避免本地数据库重复和
数据卷迁移；Redis 使用 Docker。

## 文档

- [需求规格](docs/需求规格.md)：当前基线与后续目标范围。
- [架构设计](docs/架构设计.md)：已实现链路和未实现模块的边界。
- [数据库设计](docs/数据库设计.md)：已落地表和目标表设计。
- [接口设计](docs/接口设计.md)：已实现接口与规划接口。
- [OpenAPI 3.1](web/public/openapi.yaml)：开发者接口描述。
- [开发路线](docs/开发路线.md)：里程碑、验收项和当前技术债。
- [发布验收](docs/发布验收.md)：安全、测试、备份恢复和压测结果。
- [工程协作规则](AGENTS.md)：本地资源、测试和提交约束。
- [部署与监控](deploy/README.md)：镜像、迁移、Prometheus 和 Grafana。

## 快速启动

需要：

- Go 1.26.11
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
GRAFANA_ADMIN_PASSWORD
```

### 3. 构建并启动

```powershell
.\scripts\start.ps1
```

脚本会先构建 Vue 和 `shorturl-app:local`，再启动 Redis、一次性迁移、API、
Worker、Nginx、Prometheus 和 Grafana。

Grafana 会自动加载 `Short URL Overview` 仪表盘，覆盖 HTTP 请求速率、
P95/P99 延迟、点击 Stream 长度、Pending 数量、消费速率、ACK 错误和进程内存。
登录账号来自 `deploy/.env` 中的 `GRAFANA_ADMIN_USER` 和
`GRAFANA_ADMIN_PASSWORD`。

### 数据库迁移

迁移任务固定执行 `migrate up`。Goose 通过 MySQL 中的 `goose_db_version`
记录已应用版本，重复启动时只检查版本并输出 `no migrations to run`，不会重复
执行 Up 迁移，也不会清空数据库。`migrate down` 只会在人工显式执行时发生；
生产环境还必须设置 `ALLOW_MIGRATION_DOWN=true`。

| 服务 | 地址 |
| --- | --- |
| Web 控制台 | http://localhost:18088 |
| API | http://localhost:18080 |
| Worker 健康检查 | http://localhost:18081/health/live |
| API 就绪检查 | http://localhost:18080/health/ready |
| Prometheus 指标 | Docker 内网 `api:9091` / `worker:9091`，不对宿主机发布 |
| Prometheus | http://127.0.0.1:19090 |
| Grafana | http://127.0.0.1:13000 |

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
$env:METRICS_ADDR='127.0.0.1:9091'
go run .\cmd\api
```

终端 2：

```powershell
$env:METRICS_ADDR='127.0.0.1:9092'
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
- CSV 批量创建：默认 10 次/小时/工作空间，Redis 故障时拒绝。
- 公开举报：默认 10 次/小时/IP，Redis 故障时拒绝。
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
允许 10 次创建请求，每个会话最多 3 条链接。链接按所选有效期失效，实际时间
不会超过游客会话剩余时间；会话结束后由 Worker 清理。

## 主要接口

```text
POST   /api/v1/auth/register
POST   /api/v1/auth/login
POST   /api/v1/auth/refresh

POST   /api/v1/trial/links
GET    /api/v1/trial/links
POST   /api/v1/trial/claim

GET    /api/v1/api-keys
POST   /api/v1/api-keys
DELETE /api/v1/api-keys/{id}

GET    /api/v1/webhooks
POST   /api/v1/webhooks
DELETE /api/v1/webhooks/{id}
GET    /api/v1/webhooks/{id}/deliveries

POST   /api/v1/abuse/reports
GET    /api/v1/abuse/reports
POST   /api/v1/abuse/reports/{id}/resolve
GET    /api/v1/abuse/blocked-domains
POST   /api/v1/abuse/blocked-domains
DELETE /api/v1/abuse/blocked-domains/{id}

POST   /api/v1/links
POST   /api/v1/links/bulk
GET    /api/v1/links
GET    /api/v1/links/{id}
PATCH  /api/v1/links/{id}
DELETE /api/v1/links/{id}
GET    /api/v1/links/{id}/qr
GET    /api/v1/links/{id}/analytics
GET    /api/v1/analytics/overview
GET    /api/v1/analytics/export

GET    /api/v1/domains
POST   /api/v1/domains
POST   /api/v1/domains/{id}/verify
DELETE /api/v1/domains/{id}

GET /r/{slug}       默认域名
GET /{slug}         自定义域名 Host
```

API Key 支持：

```http
Authorization: Bearer sl_live_<prefix>.<secret>
X-API-Key: sl_live_<prefix>.<secret>
```

完整 Secret 只在创建时返回一次，数据库只保存前缀和 SHA-256 哈希。作用域包括
`links:read`、`links:write` 和 `analytics:read`。API Key 和开发者页面只对
平台管理员开放；普通用户直接调用管理接口会收到 403。

平台管理员角色保存在 `users.role=2`。角色变更后需要重新登录，使新签发的
Access Token 包含管理员权限。

滥用治理：

- 访客可在 `/report` 提交钓鱼、恶意软件、垃圾推广、仿冒或其他举报。
- 管理员在 `/admin/abuse` 审核举报；确认违规时同一事务内停用短链。
- 下架提交后主动失效 Redis 跳转缓存，并记录管理员审计日志。
- 域名黑名单自动匹配精确域名及其子域名，并同时阻止后续创建和修改。

Webhook 支持 `link.created`、`link.updated` 和 `link.deleted`。创建端点时
返回一次 `whsec_...` Secret，后续请求头包含 Event ID、事件类型、Unix 时间戳
和签名。签名值为：

```text
sha256=HEX(HMAC-SHA256(secret, timestamp + "." + body))
```

只有 2xx 算成功。失败按有上限的指数退避重试，达到 `WEBHOOK_MAX_ATTEMPTS`
后终止任务并自动停用端点。管理员页面可以查看最近投递状态。

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
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...

Set-Location web
npm run typecheck
npm run build
npm audit --omit=dev --audit-level=high
```

真实 MySQL 集成测试：

```powershell
$env:SHORTURL_TEST_MYSQL_DSN='user:password@tcp(127.0.0.1:3306)/short_url?parseTime=true&loc=UTC'
go test -tags=integration ./...
```

浏览器 E2E：

```powershell
$env:SHORTURL_WEBHOOK_TARGET_HOST='127.0.0.1'
python web\scripts\e2e.py
```

覆盖免登录试用、注册认领、普通用户导航权限、后台接口 403、指标接口不可见、
短链创建、公开举报、UA 分类、访问分析、二维码下载、编辑链接和移动端布局。

## 压测结果

本机实测 500 req/s 目标，持续 20 秒：

| 指标 | 结果 |
| --- | --- |
| 跳转次数 | 10,001 |
| 实际速率 | 约 500.05 次/秒 |
| 检查成功率 | 100% |
| HTTP 请求失败 | 0 |
| P95 | 2.97 ms |
| P99 | 16.57 ms |

该结果来自单机、Redis 缓存已预热的跳转路径，用于验证当前演示版本的稳定性，
不代表生产集群容量。

详细环境、边界和原始 JSON：

- [500 req/s 报告](loadtest/reports/redirect-500rps.md)
- [收尾阶段 500 req/s 回归](loadtest/reports/redirect-closure-500rps.md)
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
internal/trial       游客会话、配额和临时链接
internal/webhook     Webhook 配置、Outbox 投递和签名
internal/abuse       滥用举报、目标黑名单和审计日志
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
- 暂未实现文件夹、标签、用户封禁和团队角色。
- 自定义域名的管理入口仍由平台域名提供；自定义域名根路径由 API 处理。
- Prometheus 指标只监听容器内网专用端口；Grafana 保留管理员登录。
