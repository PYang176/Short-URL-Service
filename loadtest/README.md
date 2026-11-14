# 跳转压测

压测目标为 Redis 缓存命中的公开跳转路径，不跟随目标页跳转。

## 前置条件

- API、Worker、Redis 和宿主机 MySQL 已启动。
- 为避免创建阶段触发限流，临时设置：

```powershell
$env:RATE_LIMIT_CREATE_LINK_LIMIT='10000'
$env:RATE_LIMIT_REDIRECT_LIMIT='1000000'
```

- 测试结束恢复默认限流。

## 运行

当前验证环境使用已有的 `grafana/k6:0.57.0` 镜像。在项目根目录启动 API、
Worker 和 Redis 后，使用 Docker Desktop 运行：

```powershell
docker run --rm `
  -e BASE_URL=http://host.docker.internal:18080 `
  -e LINK_COUNT=100 `
  -e RATE=1000 `
  -e DURATION=30s `
-e PRE_ALLOCATED_VUS=200 `
-e MAX_VUS=1000 `
  -v "${PWD}:/scripts" `
  grafana/k6:0.57.0 run `
  --summary-trend-stats "avg,min,med,max,p(90),p(95),p(99)" `
  --summary-export /scripts/loadtest/reports/redirect-1000rps.json `
  /scripts/loadtest/redirect.js
```

压测会保留测试账号和链接，结束后执行：

```powershell
mysql -u root -p short_url < loadtest\cleanup.sql
```

`RATE` 是每秒目标请求数，不是虚拟用户数。`PRE_ALLOCATED_VUS` 和 `MAX_VUS`
用于限制高目标速率下的 k6 虚拟用户数量，避免压测工具自身耗尽内存。建议依次
提高目标速率，确认错误率、延迟和 `dropped_iterations` 没有明显恶化后继续。

## 结果口径

- `http_req_failed`：错误率。
- `http_req_duration`：包含网络耗时。
- `checks`：返回状态是否符合 301/302。
- setup 和 teardown 请求带独立 tag，不计入正式 `redirect` 阈值。

必须在报告中记录机器配置、MySQL 版本、Redis 版本、API/Worker 配置和原始
JSON 文件。不要只写未经测试的 QPS 数字。

## 已生成报告

- [500 req/s 基线](./reports/redirect-500rps.md)
- [收尾阶段 500 req/s 回归](./reports/redirect-closure-500rps.md)
- [2026-11-15 容量阶梯测试](./reports/redirect-capacity-2026-11-15.md)
