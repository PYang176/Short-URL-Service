<script setup lang="ts">
import { computed } from 'vue'

import type { TrendPoint } from '@/api/types'

const props = defineProps<{
  points: TrendPoint[]
}>()

const width = 820
const height = 260
const padding = { top: 18, right: 18, bottom: 36, left: 42 }
const plotWidth = width - padding.left - padding.right
const plotHeight = height - padding.top - padding.bottom

const maxValue = computed(() =>
  Math.max(1, ...props.points.flatMap((point) => [point.human_clicks, point.bot_clicks])),
)
const yMax = computed(() => Math.max(4, Math.ceil(maxValue.value / 4) * 4))

const humanPoints = computed(() => toPoints('human_clicks'))
const botPoints = computed(() => toPoints('bot_clicks'))

const gridLines = computed(() =>
  [0, 1, 2, 3, 4].map((step) => ({
    y: padding.top + plotHeight - (yMax.value / 4) * step,
    value: (yMax.value / 4) * step,
  })),
)

const xLabels = computed(() => {
  if (props.points.length <= 7) {
    return props.points.map((point, index) => ({
      x: pointX(index),
      label: point.bucket.slice(5, 16),
    }))
  }
  const step = Math.max(1, Math.floor(props.points.length / 5))
  return props.points
    .map((point, index) => ({
      x: pointX(index),
      label: point.bucket.slice(5, 16),
      index,
    }))
    .filter((item) => item.index % step === 0 || item.index === props.points.length - 1)
})

// toPoints 将指定计数序列转换为 SVG 折线点
function toPoints(field: 'human_clicks' | 'bot_clicks'): string {
  return props.points
    .map((point, index) => `${pointX(index)},${pointY(point[field])}`)
    .join(' ')
}

// pointX 计算时间点在绘图区的横坐标
function pointX(index: number): number {
  if (props.points.length === 1) {
    return padding.left + plotWidth / 2
  }
  return padding.left + (plotWidth * index) / (props.points.length - 1)
}

// pointY 将点击量映射到绘图区纵坐标
function pointY(value: number): number {
  return padding.top + plotHeight - (value / yMax.value) * plotHeight
}
</script>

<template>
  <svg
    class="trend-chart"
    :viewBox="`0 0 ${width} ${height}`"
    role="img"
    :aria-label="`点击趋势，共 ${points.length} 个时间点`"
  >
    <title>短链点击趋势</title>
    <desc>实线表示未命中 User-Agent 规则的点击，虚线表示疑似机器人点击。</desc>
    <g aria-hidden="true">
      <line
        v-for="line in gridLines"
        :key="line.value"
        :x1="padding.left"
        :x2="width - padding.right"
        :y1="line.y"
        :y2="line.y"
        stroke="#e6ecea"
      />
      <text
        v-for="line in gridLines"
        :key="`label-${line.value}`"
        :x="padding.left - 8"
        :y="line.y + 4"
        text-anchor="end"
        fill="#647572"
        font-size="11"
      >
        {{ line.value }}
      </text>
      <text
        v-for="label in xLabels"
        :key="`${label.x}-${label.label}`"
        :x="label.x"
        :y="height - 12"
        text-anchor="middle"
        fill="#647572"
        font-size="10"
      >
        {{ label.label }}
      </text>
      <polyline
        :points="humanPoints"
        fill="none"
        stroke="#167c6c"
        stroke-width="3"
        stroke-linecap="round"
        stroke-linejoin="round"
      />
      <polyline
        :points="botPoints"
        fill="none"
        stroke="#c26848"
        stroke-width="2"
        stroke-dasharray="7 5"
        stroke-linecap="round"
        stroke-linejoin="round"
      />
      <circle
        v-for="(point, index) in points"
        :key="`human-${point.bucket}`"
        :cx="pointX(index)"
        :cy="pointY(point.human_clicks)"
        r="4"
        fill="#ffffff"
        stroke="#167c6c"
        stroke-width="2.5"
      />
      <circle
        v-for="(point, index) in points"
        :key="`bot-${point.bucket}`"
        :cx="pointX(index)"
        :cy="pointY(point.bot_clicks)"
        r="3"
        fill="#c26848"
      />
    </g>
  </svg>
</template>
