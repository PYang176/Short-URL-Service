"""根据 k6 原始摘要生成 README 使用的跳转容量曲线图。"""

from __future__ import annotations

import json
from pathlib import Path

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt
from matplotlib.ticker import FuncFormatter


ROOT = Path(__file__).resolve().parent.parent
REPORT_DIR = ROOT / "loadtest" / "reports"
OUTPUT_PATH = ROOT / "docs" / "assets" / "loadtest" / "redirect-capacity.png"
RATES = (500, 1000, 2000, 3000, 4000, 4500, 5000)


# load_metrics 读取每个目标速率对应的 k6 JSON 摘要。
def load_metrics() -> tuple[list[float], list[float], list[float]]:
    actual: list[float] = []
    p95: list[float] = []
    p99: list[float] = []
    for rate in RATES:
        report = REPORT_DIR / f"redirect-capacity-{rate}rps.json"
        if not report.exists():
            raise FileNotFoundError(f"缺少压测结果: {report}")
        metrics = json.loads(report.read_text(encoding="utf-8"))["metrics"]
        duration = metrics["http_req_duration"]
        actual.append(float(metrics["iterations"]["rate"]))
        p95.append(float(duration["p(95)"]))
        p99.append(float(duration["p(99)"]))
    return actual, p95, p99


# format_rate 将速率格式化为千位分隔的整数。
def format_rate(value: float, _position: int) -> str:
    return f"{value:,.0f}"


# render_chart 绘制吞吐与尾延迟双面板图，并写入固定输出路径。
def render_chart() -> None:
    actual, p95, p99 = load_metrics()
    plt.rcParams["font.sans-serif"] = ["Microsoft YaHei", "DejaVu Sans"]
    plt.rcParams["axes.unicode_minus"] = False

    figure = plt.figure(figsize=(14, 6.4), dpi=180, facecolor="#fbfcfc")
    grid = figure.add_gridspec(1, 2, width_ratios=(1.15, 1), wspace=0.24)
    throughput_axis = figure.add_subplot(grid[0, 0])
    latency_axis = figure.add_subplot(grid[0, 1])

    stable_color = "#167c6c"
    saturation_color = "#c26848"
    grid_color = "#dce5e2"
    text_color = "#24332f"
    muted_color = "#687a75"

    throughput_axis.set_facecolor("#ffffff")
    throughput_axis.plot(
        RATES,
        RATES,
        color="#9cafaa",
        linewidth=1.2,
        linestyle=(0, (4, 4)),
        label="目标速率",
    )
    throughput_axis.plot(
        RATES[:-1],
        actual[:-1],
        color=stable_color,
        linewidth=2.4,
        marker="o",
        markersize=5,
        label="实际完成",
    )
    throughput_axis.plot(
        RATES[-2:],
        actual[-2:],
        color=saturation_color,
        linewidth=2.4,
        marker="o",
        markersize=6,
    )
    throughput_axis.axhline(
        4650,
        color=saturation_color,
        linewidth=1,
        linestyle=(0, (3, 4)),
        alpha=0.7,
    )
    throughput_axis.annotate(
        "观察吞吐上限 ≈ 4,650/s",
        xy=(3150, 4650),
        xytext=(1850, 4250),
        color=saturation_color,
        fontsize=10,
        arrowprops={"arrowstyle": "-", "color": saturation_color, "lw": 1},
    )
    throughput_axis.annotate(
        "5,000 目标\n4,647 完成\n77/s 未及时发起",
        xy=(5000, actual[-1]),
        xytext=(3720, 3300),
        color=saturation_color,
        fontsize=9,
        ha="left",
        arrowprops={"arrowstyle": "-", "color": saturation_color, "lw": 1},
    )
    throughput_axis.set_title("实际吞吐随目标速率变化", loc="left", fontsize=13, pad=12)
    throughput_axis.set_xlabel("目标速率（req/s）")
    throughput_axis.set_ylabel("实际 iteration 速率（次/秒）")
    throughput_axis.set_xlim(250, 5250)
    throughput_axis.set_ylim(0, 5250)
    throughput_axis.xaxis.set_major_formatter(FuncFormatter(format_rate))
    throughput_axis.yaxis.set_major_formatter(FuncFormatter(format_rate))
    throughput_axis.grid(axis="y", color=grid_color, linewidth=0.8)
    throughput_axis.legend(frameon=False, loc="upper left")

    latency_axis.set_facecolor("#ffffff")
    latency_axis.plot(
        RATES,
        p95,
        color=stable_color,
        linewidth=2.2,
        marker="o",
        markersize=5,
        label="P95",
    )
    latency_axis.plot(
        RATES,
        p99,
        color=saturation_color,
        linewidth=1.8,
        marker="s",
        markersize=4,
        label="P99",
    )
    latency_axis.axhline(50, color=muted_color, linewidth=1, linestyle=(0, (3, 4)))
    latency_axis.axhline(100, color=muted_color, linewidth=1, linestyle=(0, (3, 4)))
    latency_axis.set_yscale("log")
    latency_axis.set_title("尾延迟随负载变化", loc="left", fontsize=13, pad=12)
    latency_axis.set_xlabel("目标速率（req/s）")
    latency_axis.set_ylabel("请求耗时（ms，对数刻度）")
    latency_axis.set_xlim(250, 5250)
    latency_axis.set_ylim(0.8, 300)
    latency_axis.xaxis.set_major_formatter(FuncFormatter(format_rate))
    latency_axis.yaxis.set_major_formatter(FuncFormatter(lambda value, _: f"{value:g}"))
    latency_axis.grid(axis="y", color=grid_color, linewidth=0.8)
    latency_axis.annotate(
        "P95 110.49 ms\nP99 134.70 ms",
        xy=(5000, p95[-1]),
        xytext=(3650, 145),
        color=saturation_color,
        fontsize=9,
        arrowprops={"arrowstyle": "-", "color": saturation_color, "lw": 1},
    )
    latency_axis.legend(frameon=False, loc="center left")

    for axis in (throughput_axis, latency_axis):
        axis.spines[["top", "right"]].set_visible(False)
        axis.spines[["left", "bottom"]].set_color("#b8c8c3")
        axis.tick_params(colors=muted_color, labelsize=9)
        axis.title.set_color(text_color)
        axis.xaxis.label.set_color(muted_color)
        axis.yaxis.label.set_color(muted_color)

    figure.suptitle(
        "单实例短链跳转容量与尾延迟",
        x=0.07,
        y=0.98,
        ha="left",
        fontsize=17,
        fontweight="bold",
        color=text_color,
    )
    figure.text(
        0.07,
        0.925,
        "Redis 缓存命中 · 20 条预热短链 · API、Worker、Redis、MySQL 与 k6 同机运行",
        ha="left",
        fontsize=9.5,
        color=muted_color,
    )
    figure.subplots_adjust(left=0.07, right=0.965, bottom=0.12, top=0.86)
    OUTPUT_PATH.parent.mkdir(parents=True, exist_ok=True)
    figure.savefig(OUTPUT_PATH, facecolor=figure.get_facecolor())
    plt.close(figure)
    print(OUTPUT_PATH)


if __name__ == "__main__":
    render_chart()
