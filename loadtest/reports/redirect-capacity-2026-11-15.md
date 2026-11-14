# 短链跳转容量压测报告

测试日期：2026-11-15

## 1. 结论

本次测试覆盖 Redis 缓存已预热的公开跳转路径。当前单机、单 API 实例配置下：

- 4,500 req/s 目标可以稳定完成，未出现 HTTP 错误或未发起迭代。
- 约 4,650 req/s 是本次环境观察到的吞吐上限。
- 5,000 req/s 目标无法持续维持，P95 升至 110.49 ms，并有 77.03 次/秒的计划
  iteration 未能及时发起。
- 面向容量规划，建议将当前单实例演示环境控制在 3,000 至 4,000 req/s，
  为 GC、Worker 消费和宿主机资源波动保留余量。

以上结果是本机单实例、缓存命中和同机压测结果，不代表生产集群容量。

## 2. 测试环境

| 项目 | 配置 |
| --- | --- |
| CPU | AMD Ryzen 7 5800H，8 核 16 线程 |
| 操作系统 | Windows 11 |
| Docker 内存上限 | 15.54 GiB |
| API | `shorturl-app:local`，单实例，隔离压测容器 |
| Worker | `shorturl-app:local`，单实例 |
| MySQL | 8.0.34，宿主机原生进程 |
| Redis | 7.4 Alpine，Docker 容器 |
| k6 | `grafana/k6:0.57.0`，Docker 容器 |
| 短链数量 | 每个场景 20 条，正式压测前完成缓存预热 |
| API 限流 | 隔离压测实例关闭限流 |

API、Worker、Redis、MySQL 和 k6 运行在同一台机器上，CPU 与网络存在资源竞争。

## 3. 测试方法

每个场景执行以下步骤：

```text
创建独立压测账号
  -> 创建 20 条短链
  -> 每条短链访问一次，预热 Redis
  -> constant-arrival-rate 阶梯加压
  -> 请求 /r/{slug}，不跟随目标页跳转
  -> 检查 301/302、错误率、P95、P99 和 dropped_iterations
```

验收阈值：

```text
http_req_failed < 1%
http_req_duration p(95) < 50 ms
http_req_duration p(99) < 100 ms
checks > 99%
```

500 至 3,000 req/s 使用 15 秒场景，4,000 和 4,500 req/s 使用 20 秒场景。

## 4. 阶梯结果

| 目标速率 | 实际 iteration 速率 | 未发起 iteration | 错误率 | P95 | P99 | 结果 |
| ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 500/s | 482.13/s | 0 | 0% | 1.29 ms | 1.69 ms | 通过 |
| 1,000/s | 965.77/s | 0 | 0% | 1.23 ms | 1.93 ms | 通过 |
| 2,000/s | 1,934.04/s | 0 | 0% | 1.38 ms | 3.94 ms | 通过 |
| 3,000/s | 2,898.98/s | 0 | 0% | 1.43 ms | 2.88 ms | 通过 |
| 4,000/s | 3,895.52/s | 0 | 0% | 4.83 ms | 12.48 ms | 通过 |
| 4,500/s | 4,371.29/s | 0 | 0% | 2.09 ms | 6.23 ms | 通过 |
| 5,000/s | 4,646.66/s | 1,223（77.03/s） | 0% | 110.49 ms | 134.70 ms | 延迟阈值失败 |

4,500 req/s 的 P95 低于 4,000 req/s，属于 GC 调度和宿主机资源波动，不应理解为
高负载下延迟必然下降。

## 5. 资源使用

Docker 的 CPU 百分比以单核为 100% 采样：

| 场景 | API CPU 峰值 | Redis CPU 峰值 | Worker CPU 峰值 |
| --- | ---: | ---: | ---: |
| 4,000/s 稳定场景 | 约 326% | 约 87% | 约 37% |
| 5,000/s 饱和场景 | 约 701% | 约 114% | 约 34% |

4,000 req/s 时 API 尚有余量；提升到 5,000 req/s 后 API 接近 7 个 CPU 核，
Redis 也超过 1 个 CPU 核，说明继续加压主要受本机 CPU 和 Redis 处理能力限制。
API 内存采样峰值约 176 MiB，未观察到持续增长。

## 6. 异步统计链路

全部场景结束后检查 Redis 点击消费组：

```text
stream length: 351648
pending: 0
lag: 0
```

Worker 能够跟上压测产生的点击事件，未发现 Pending 或消费积压。

## 7. 结果边界

- 测试的是 Redis 缓存命中的公开跳转热路径，不代表缓存未命中或管理接口容量。
- 不跟随 302 目标页，不包含目标网站响应时间。
- 请求直接进入隔离 API 容器，不包含公网 DNS、TLS 和真实网络链路损耗。
- k6 与 API 运行在同一台机器上，压测工具本身也消耗 CPU。
- 5,000 req/s 场景没有 HTTP 错误，失败指标来自 k6 无法按计划发起全部请求，
  不等同于网关返回 500 或 503。
- 当前结果不能直接推导多实例、生产 Redis 或云主机容量。

## 8. 原始结果

- [500 req/s JSON](./redirect-capacity-500rps.json)
- [1,000 req/s JSON](./redirect-capacity-1000rps.json)
- [2,000 req/s JSON](./redirect-capacity-2000rps.json)
- [3,000 req/s JSON](./redirect-capacity-3000rps.json)
- [4,000 req/s JSON](./redirect-capacity-4000rps.json)
- [4,500 req/s JSON](./redirect-capacity-4500rps.json)
- [5,000 req/s JSON](./redirect-capacity-5000rps.json)
