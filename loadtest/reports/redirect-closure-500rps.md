# 收尾阶段 500 req/s 跳转压测

## 目标

验证当前收尾版本在 Redis 缓存命中路径上的跳转稳定性。请求不跟随目标页跳转，
只统计短链服务返回的 301/302。

## 环境

| 项目 | 值 |
| --- | --- |
| 操作系统 | Windows NT 10.0.26200.0 |
| CPU | AMD Ryzen 7 5800H，8 核 16 线程 |
| k6 | `grafana/k6:0.57.0`，Docker Desktop |
| Go | 1.26.11 |
| API / Worker | `shorturl-app:local` |
| MySQL | 8.0.34，宿主机实例 |
| Redis | 7.4 Alpine，Docker 内部网络 |
| 链接 | 100 条，正式压测前完成缓存预热 |

## 参数

```text
executor: constant-arrival-rate
rate: 500 iterations/s
duration: 20s
maxVUs: 1000
```

## 结果

| 指标 | 结果 |
| --- | --- |
| 跳转 iteration | 10,001 次 |
| 跳转速率 | 约 500.05 次/秒 |
| 检查成功率 | 100% |
| HTTP 请求失败 | 0 |
| P95 | 2.97 ms |
| P99 | 16.57 ms |
| 平均延迟 | 2.69 ms |
| 最大延迟 | 120.91 ms |

总运行时间约 23.4 秒，包含注册、创建 100 条链接和缓存预热。`http_reqs`
汇总指标因此包含 setup 请求，不能用它反推正式跳转阶段速率。

## 原始结果

- [JSON 原始结果](./redirect-closure-500rps.json)
