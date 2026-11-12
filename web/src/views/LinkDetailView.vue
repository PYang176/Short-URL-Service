<script setup lang="ts">
import { computed, onBeforeUnmount, shallowRef, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'

import { getAnalytics, getLink, getQRCode } from '@/api'
import { ApiError } from '@/api/http'
import type { AnalyticsOverview, Link } from '@/api/types'
import BreakdownList from '@/components/BreakdownList.vue'
import LinkFormDialog from '@/components/LinkFormDialog.vue'
import TrendChart from '@/components/TrendChart.vue'
import { formatCompactNumber, formatDateTime, statusLabel } from '@/utils/format'

const route = useRoute()
const link = shallowRef<Link | null>(null)
const analytics = shallowRef<AnalyticsOverview | null>(null)
const days = shallowRef(7)
const loading = shallowRef(true)
const analyticsLoading = shallowRef(false)
const errorMessage = shallowRef('')
const editOpen = shallowRef(false)
const qrURL = shallowRef('')
const qrBlob = shallowRef<Blob | null>(null)
const notice = shallowRef('')

const humanRate = computed(() => {
  const summary = analytics.value?.summary
  if (!summary || summary.total_clicks === 0) {
    return 0
  }
  return Math.round((summary.human_clicks / summary.total_clicks) * 100)
})

// loadAll 加载链接详情、二维码和分析
async function loadAll(): Promise<void> {
  const id = String(route.params.id)
  loading.value = true
  errorMessage.value = ''
  try {
    const [linkResult, qrResult] = await Promise.all([
      getLink(id),
      getQRCode(id, 'png'),
    ])
    link.value = linkResult
    updateQRCode(qrResult)
    await loadAnalytics()
  } catch (error) {
    errorMessage.value = error instanceof ApiError ? error.message : '加载链接详情失败'
  } finally {
    loading.value = false
  }
}

// loadAnalytics 加载当前时间范围的分析
async function loadAnalytics(): Promise<void> {
  const id = String(route.params.id)
  analyticsLoading.value = true
  try {
    analytics.value = await getAnalytics(id, days.value)
  } catch (error) {
    notice.value = error instanceof ApiError ? error.message : '加载分析失败'
  } finally {
    analyticsLoading.value = false
  }
}

// updateQRCode 替换二维码预览对象 URL
function updateQRCode(blob: Blob): void {
  if (qrURL.value) {
    URL.revokeObjectURL(qrURL.value)
  }
  qrBlob.value = blob
  qrURL.value = URL.createObjectURL(blob)
}

// copyLink 复制当前短链
async function copyLink(): Promise<void> {
  if (!link.value) {
    return
  }
  try {
    await navigator.clipboard.writeText(link.value.short_url)
    notice.value = '短链地址已复制'
  } catch {
    notice.value = '浏览器未允许复制'
  }
}

// downloadQR 下载指定格式的二维码
async function downloadQR(format: 'png' | 'svg'): Promise<void> {
  if (!link.value) {
    return
  }
  let blob = format === 'png' ? qrBlob.value : null
  if (!blob) {
    try {
      blob = await getQRCode(link.value.id, format)
    } catch (error) {
      notice.value = error instanceof ApiError ? error.message : '下载二维码失败'
      return
    }
  }
  const anchor = document.createElement('a')
  const url = URL.createObjectURL(blob)
  anchor.href = url
  anchor.download = `${link.value.slug}.${format}`
  anchor.click()
  URL.revokeObjectURL(url)
}

// handleSaved 更新编辑后的详情数据
async function handleSaved(updated: Link): Promise<void> {
  link.value = updated
  editOpen.value = false
  notice.value = '链接已更新'
  await loadAnalytics()
}

watch(
  () => route.params.id,
  () => {
    void loadAll()
  },
  { immediate: true },
)

watch(days, () => {
  void loadAnalytics()
})

onBeforeUnmount(() => {
  if (qrURL.value) {
    URL.revokeObjectURL(qrURL.value)
  }
})
</script>

<template>
  <div class="page-stack">
    <div class="detail-back">
      <RouterLink class="text-link" to="/links">返回短链接列表</RouterLink>
    </div>

    <div v-if="loading" class="skeleton-stack" role="status" aria-label="正在加载链接详情">
      <span class="skeleton-hero" />
      <span class="skeleton-row" />
      <span class="skeleton-row" />
    </div>
    <div v-else-if="errorMessage" class="error-panel" role="alert">
      <p>{{ errorMessage }}</p>
      <button class="secondary-button" type="button" @click="loadAll">重新加载</button>
    </div>
    <template v-else-if="link">
      <header class="detail-header">
        <div class="detail-heading">
          <div class="detail-title-line">
            <span class="status-pill" :class="{ 'is-disabled': link.status !== 1 }">
              {{ statusLabel(link.status) }}
            </span>
            <code>/{{ link.slug }}</code>
          </div>
          <h1>{{ link.title || '未命名短链接' }}</h1>
          <a :href="link.destination_url" target="_blank" rel="noreferrer" class="destination-link">
            {{ link.destination_url }}
          </a>
        </div>
        <div class="detail-actions">
          <button class="secondary-button" type="button" @click="copyLink">复制短链</button>
          <button class="primary-button" type="button" @click="editOpen = true">编辑链接</button>
        </div>
      </header>

      <p v-if="notice" class="inline-notice" role="status">{{ notice }}</p>

      <section class="detail-summary">
        <div class="summary-primary">
          <span>总点击</span>
          <strong>{{ formatCompactNumber(analytics?.summary.total_clicks ?? 0) }}</strong>
          <small>{{ days }} 天范围</small>
        </div>
        <div class="summary-cell">
          <span>未命中 UA 规则</span>
          <strong>{{ formatCompactNumber(analytics?.summary.human_clicks ?? 0) }}</strong>
          <small>占 {{ humanRate }}%</small>
        </div>
        <div class="summary-cell">
          <span>疑似机器人</span>
          <strong>{{ formatCompactNumber(analytics?.summary.bot_clicks ?? 0) }}</strong>
          <small>仅按 UA 子串规则判断</small>
        </div>
        <div class="summary-cell">
          <span>独立访客</span>
          <strong>{{ formatCompactNumber(analytics?.summary.unique_visitors ?? 0) }}</strong>
          <small>同一访客哈希去重</small>
        </div>
      </section>

      <div class="detail-grid">
        <section class="panel trend-panel">
          <div class="section-heading-row">
            <div>
              <h2>点击趋势</h2>
              <p>点击从 Redis Stream 异步写入 MySQL。</p>
            </div>
            <label class="compact-field">
              <span>范围</span>
              <select v-model="days">
                <option :value="1">1 天</option>
                <option :value="7">7 天</option>
                <option :value="30">30 天</option>
                <option :value="90">90 天</option>
              </select>
            </label>
          </div>
          <div v-if="analyticsLoading" class="chart-placeholder" role="status">
            正在刷新分析…
          </div>
          <p v-else-if="!analytics?.trend.length" class="quiet-state">
            还没有点击记录。打开一次短链后再回来查看。
          </p>
          <TrendChart v-else :points="analytics.trend" />
        </section>

        <aside class="qr-card">
          <div class="section-heading-row">
            <div>
              <h2>动态二维码</h2>
              <p>二维码指向当前短链，不随目标修改失效。</p>
            </div>
          </div>
          <div class="qr-preview">
            <img v-if="qrURL" :src="qrURL" :alt="`短链接 /${link.slug} 的二维码`" />
          </div>
          <div class="qr-actions">
            <button class="primary-button" type="button" @click="downloadQR('png')">
              下载 PNG
            </button>
            <button class="secondary-button" type="button" @click="downloadQR('svg')">
              下载 SVG
            </button>
            <a class="secondary-button" :href="link.short_url" target="_blank" rel="noreferrer">
              打开短链
            </a>
          </div>
          <dl class="detail-facts">
            <div><dt>跳转方式</dt><dd>{{ link.redirect_code }}</dd></div>
            <div><dt>创建时间</dt><dd>{{ formatDateTime(link.created_at) }}</dd></div>
            <div><dt>过期时间</dt><dd>{{ formatDateTime(link.expires_at) }}</dd></div>
          </dl>
        </aside>
      </div>

      <section class="analytics-grid" aria-label="点击维度分布">
        <BreakdownList title="设备" :items="analytics?.devices ?? []" />
        <BreakdownList title="浏览器" :items="analytics?.browsers ?? []" />
        <BreakdownList title="操作系统" :items="analytics?.operating_systems ?? []" />
        <BreakdownList title="国家或地区" :items="analytics?.countries ?? []" />
        <BreakdownList title="来源页面" :items="analytics?.referrers ?? []" />
      </section>

      <LinkFormDialog
        :open="editOpen"
        :link="link"
        @close="editOpen = false"
        @saved="handleSaved"
      />
    </template>
  </div>
</template>
