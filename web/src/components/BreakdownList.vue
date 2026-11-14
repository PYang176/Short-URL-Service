<script setup lang="ts">
import { computed } from 'vue'

import type { BreakdownItem } from '@/api/types'

const props = defineProps<{
  title: string
  items: BreakdownItem[]
  emptyText?: string
}>()

const maxClicks = computed(() =>
  Math.max(1, ...props.items.map((item) => item.clicks)),
)

const displayEmptyText = computed(() => props.emptyText ?? '暂无数据')

// barWidth 将点击量转换为相对宽度
function barWidth(clicks: number): string {
  return `${Math.max(4, (clicks / maxClicks.value) * 100)}%`
}

// displayValue 将常见内部枚举转换为面向用户的简短文案。
function displayValue(value: string): string {
  const labels: Record<string, string> = {
    desktop: '桌面设备',
    mobile: '手机',
    tablet: '平板',
    unknown: '未知',
    direct: '直接访问',
  }
  return labels[value] ?? value
}
</script>

<template>
  <section class="breakdown-card">
    <h3 class="panel-title">{{ title }}</h3>
    <p v-if="items.length === 0" class="quiet-state">{{ displayEmptyText }}</p>
    <ol v-else class="breakdown-list">
      <li v-for="item in items" :key="item.value" class="breakdown-item">
        <div class="breakdown-meta">
          <span>{{ displayValue(item.value) }}</span>
          <strong>{{ item.clicks }}</strong>
        </div>
        <div class="breakdown-track" aria-hidden="true">
          <span class="breakdown-fill" :style="{ width: barWidth(item.clicks) }" />
        </div>
      </li>
    </ol>
  </section>
</template>
