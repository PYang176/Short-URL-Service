# 部署与监控

## 组件

- `shorturl-app:local`：包含 API、Worker 和迁移二进制，通过不同命令运行。
- `shorturl-web:latest`：Nginx 托管 Vue 构建产物并代理 API。
- Prometheus：抓取 API 和 Worker 的 `/metrics`。
- Grafana：自动加载 Prometheus 数据源和 `Short URL Overview` 仪表盘。
- MySQL：宿主机实例，不放入 Compose。

## 启动

```powershell
Copy-Item deploy\.env.example deploy\.env
notepad deploy\.env
.\scripts\start.ps1
```

必须配置：

```text
MYSQL_DSN
JWT_SECRET
ANALYTICS_HASH_SECRET
GRAFANA_ADMIN_PASSWORD
```

监控地址：

- Prometheus：http://127.0.0.1:19090
- Grafana：http://127.0.0.1:13000
- API 指标：http://127.0.0.1:18080/metrics
- Worker 指标：http://127.0.0.1:18081/metrics

## 仪表盘

`grafana/dashboards/shorturl-overview.json` 包含：

- HTTP 请求速率和状态码分布。
- HTTP P95/P99 延迟。
- Redis Stream 长度和 Pending 数量。
- 点击消费成功、失败和无效消息速率。
- ACK 错误。
- API 与 Worker 进程常驻内存。

仪表盘只读 provisioning，Git 中的 JSON 是唯一配置来源。

## 迁移

迁移容器只执行 `migrate up`。Goose 使用 `goose_db_version` 跳过已经应用的
版本，重复启动不会重复执行迁移。生产环境执行 `migrate down` 必须显式设置：

```text
ALLOW_MIGRATION_DOWN=true
```
