# 短链跳转 500 req/s 压测报告

测试日期：2026-11-13

## 1. 测试结论

在本机单实例环境下，Redis 缓存命中的公开跳转路径完成 10,000 次请求：

- 目标速率：500 req/s。
- 实际迭代速率：482.89 req/s。
- 错误率：0%。
- P95：3.55ms。
- P99：4.99ms。
- 最大请求耗时：74.31ms。

本次结果证明当前单机链路和验收脚本可运行，不代表生产集群容量。

## 2. 测试环境

| 项目 | 配置 |
| --- | --- |
| CPU | AMD Ryzen 7 5800H，8 核 16 线程 |
| 内存 | 约 32 GB |
| 操作系统 | Windows 11 家庭版 10.0.26200 |
| Go | 1.26 |
| MySQL | 8.0.34，宿主机原生进程 |
| Redis | 7.4-alpine Docker 容器 |
| API | `go run ./cmd/api`，单实例 |
| Worker | `go run ./cmd/worker`，单实例 |
| 压测工具 | `grafana/k6:0.57.0` Docker 容器 |

API、Worker、MySQL、Redis 和 k6 运行在同一台机器上。

## 3. 测试方法

```text
创建 1 个压测账号
  -> 创建 50 个随机短链
  -> 每链接预热一次 Redis
  -> constant-arrival-rate 500 req/s
  -> 持续 20 秒
  -> 请求 /r/{slug}，不跟随目标页跳转
```

k6 脚本：

- [redirect.js](../redirect.js)

原始摘要：

- [redirect-500rps.json](./redirect-500rps.json)

运行参数：

```text
LINK_COUNT=50
RATE=500
DURATION=20s
```

## 4. 指标

| 指标 | 结果 |
| --- | --- |
| 跳转迭代数 | 10,000 |
| 总 HTTP 请求数 | 10,101 |
| 实际迭代速率 | 482.89 req/s |
| 实际 HTTP 请求速率 | 487.77 req/s |
| `checks` | 100%（10,000 / 10,000） |
| `http_req_failed` | 0% |
| 平均延迟 | 3.14ms |
| P90 | 3.40ms |
| P95 | 3.55ms |
| P99 | 4.99ms |
| 最大延迟 | 74.31ms |
| 活跃 VUs | 最大 2 |

阈值：

```text
http_req_failed rate < 1%
http_req_duration p(95) < 50ms
http_req_duration p(99) < 100ms
checks rate > 99%
```

全部通过。

## 5. 结果边界

- 所有链接已预热，测试的是 Redis 缓存命中路径。
- 302 目标为 `example.com`，k6 不跟随跳转，不包含目标网站响应时间。
- 点击事件同步写入 Redis Stream，但点击统计写入 MySQL 在 Worker 中异步执行。
- 同一台机器同时运行 API、Worker、Redis、MySQL 和 k6，存在资源竞争。
- 单实例结果不能外推为多实例或生产 SLA。
- 500 req/s 是本次配置；不能仅凭此次结果宣称支持任意并发。
