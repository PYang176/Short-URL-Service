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
</script>

<template>
  <section class="breakdown-card">
    <h3 class="panel-title">{{ title }}</h3>
    <p v-if="items.length === 0" class="quiet-state">{{ displayEmptyText }}</p>
    <ol v-else class="breakdown-list">
      <li v-for="item in items" :key="item.value" class="breakdown-item">
        <div class="breakdown-meta">
          <span>{{ item.value }}</span>
          <strong>{{ item.clicks }}</strong>
        </div>
        <div class="breakdown-track" aria-hidden="true">
          <span class="breakdown-fill" :style="{ width: barWidth(item.clicks) }" />
        </div>
      </li>
    </ol>
  </section>
</template>
