<script setup lang="ts">
import { computed, onMounted, shallowRef, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { RouterLink } from 'vue-router'

import { getWorkspaceAnalytics } from '@/api'
import { ApiError } from '@/api/http'
import type { WorkspaceAnalyticsOverview } from '@/api/types'
import LinkFormDialog from '@/components/LinkFormDialog.vue'
import TrendChart from '@/components/TrendChart.vue'
import { useLinks } from '@/composables/useLinks'
import { useAuthStore } from '@/stores/auth'
import {
  formatCompactNumber,
  formatDateTime,
  shortDestination,
  statusLabel,
} from '@/utils/format'

const auth = useAuthStore()
const { user, workspace } = storeToRefs(auth)
const { links, loading, errorMessage, load } = useLinks(8)
const createOpen = shallowRef(false)
const claimNotice = shallowRef(auth.consumeTrialClaimNotice())
const overview = shallowRef<WorkspaceAnalyticsOverview | null>(null)
const overviewDays = shallowRef(7)
const overviewLoading = shallowRef(true)
const overviewError = shallowRef('')
const recentLinks = computed(() => links.value.slice(0, 5))
const topLinks = computed(() => overview.value?.top_links ?? [])
const trendPoints = computed(() => overview.value?.trend ?? [])

onMounted(load)

// loadOverview 加载工作空间整体指标、趋势和热门链接
async function loadOverview(): Promise<void> {
  overviewLoading.value = true
  overviewError.value = ''
  try {
    overview.value = await getWorkspaceAnalytics(overviewDays.value)
  } catch (error) {
    overviewError.value =
      error instanceof ApiError ? error.message : '加载工作空间分析失败'
  } finally {
    overviewLoading.value = false
  }
}

watch(overviewDays, () => void loadOverview(), { immediate: true })

// handleCreated 创建后刷新工作台链接
async function handleCreated(): Promise<void> {
  createOpen.value = false
  await Promise.all([load(), loadOverview()])
}
</script>

<template>
  <div class="page-stack">
    <header class="dashboard-hero">
      <div class="hero-copy">
        <p class="section-kicker">{{ workspace?.name }}</p>
        <h1>{{ user?.display_name }}，今天要发送什么链接？</h1>
        <p class="hero-description">
          创建可追踪的短链接，随后在详情页查看点击趋势、来源设备和二维码。
        </p>
        <button class="primary-button" type="button" @click="createOpen = true">
          创建短链接
        </button>
      </div>

      <div class="route-inspector" aria-label="短链接处理流程">
        <div class="route-track">
          <span class="route-node route-node-source" />
          <span class="route-line" />
          <span class="route-node route-node-code">/{slug}</span>
          <span class="route-line" />
          <span class="route-node route-node-target" />
        </div>
        <div class="route-labels">
          <span>源地址</span>
          <span>缓存跳转</span>
          <span>访问分析</span>
        </div>
      </div>
    </header>

    <p v-if="claimNotice" class="inline-notice" role="status">{{ claimNotice }}</p>

    <section class="metric-strip" aria-label="工作空间整体概况">
      <div class="metric-cell">
        <span>全部链接</span>
        <strong>{{ overviewLoading ? '—' : (overview?.summary.total_links ?? 0) }}</strong>
      </div>
      <div class="metric-cell">
        <span>当前启用</span>
        <strong>{{ overviewLoading ? '—' : (overview?.summary.active_links ?? 0) }}</strong>
      </div>
      <div class="metric-cell">
        <span>{{ overviewDays }} 天点击</span>
        <strong>
          {{ overviewLoading ? '—' : formatCompactNumber(overview?.summary.total_clicks ?? 0) }}
        </strong>
      </div>
      <div class="metric-cell">
        <span>独立访客</span>
        <strong>
          {{
            overviewLoading
              ? '—'
              : formatCompactNumber(overview?.summary.unique_visitors ?? 0)
          }}
        </strong>
      </div>
    </section>

    <div class="dashboard-overview-grid">
      <section class="panel overview-trend-panel">
        <div class="section-heading-row">
          <div>
            <h2>整体点击趋势</h2>
            <p>聚合当前工作空间的全部短链点击。</p>
          </div>
          <label class="compact-field">
            <span>范围</span>
            <select v-model="overviewDays">
              <option :value="7">7 天</option>
              <option :value="30">30 天</option>
              <option :value="90">90 天</option>
            </select>
          </label>
        </div>

        <div v-if="overviewLoading" class="chart-placeholder" role="status">
          正在加载整体趋势…
        </div>
        <div v-else-if="overviewError" class="error-panel" role="alert">
          <p>{{ overviewError }}</p>
          <button class="secondary-button" type="button" @click="loadOverview">
            重新加载
          </button>
        </div>
        <p v-else-if="!trendPoints.length" class="quiet-state">
          当前范围内还没有点击记录。
        </p>
        <TrendChart v-else :points="trendPoints" />
      </section>

      <section class="panel overview-top-panel">
        <div class="section-heading-row">
          <div>
            <h2>热门短链</h2>
            <p>按当前时间范围内的点击量排序。</p>
          </div>
        </div>

        <p v-if="overviewLoading" class="quiet-state" role="status">正在统计…</p>
        <p v-else-if="!topLinks.length" class="quiet-state">
          创建并分享短链后，这里会出现点击排行。
        </p>
        <ol v-else class="workspace-top-list">
          <li v-for="item in topLinks" :key="item.id">
            <RouterLink :to="`/links/${item.id}`" class="workspace-top-row">
              <span class="workspace-top-title">
                <strong>{{ item.title || `/${item.slug}` }}</strong>
                <span>/{{ item.slug }}</span>
              </span>
              <span class="workspace-top-clicks">
                {{ formatCompactNumber(item.clicks) }} 次
              </span>
            </RouterLink>
          </li>
        </ol>
      </section>
    </div>

    <section class="panel">
      <div class="section-heading-row">
        <div>
          <h2>最近创建</h2>
          <p>快速回到最近使用的跳转入口。</p>
        </div>
        <RouterLink class="text-link" to="/links">查看全部短链接</RouterLink>
      </div>

      <div v-if="loading" class="skeleton-stack" role="status" aria-label="正在加载链接">
        <span v-for="index in 4" :key="index" class="skeleton-row" />
      </div>
      <div v-else-if="errorMessage" class="error-panel" role="alert">
        <p>{{ errorMessage }}</p>
        <button class="secondary-button" type="button" @click="load">重新加载</button>
      </div>
      <div v-else-if="recentLinks.length === 0" class="empty-state">
        <h3>还没有短链接</h3>
        <p>创建第一个链接后，这里会显示最近使用的入口。</p>
        <button class="primary-button" type="button" @click="createOpen = true">
          创建第一条短链
        </button>
      </div>
      <div v-else class="recent-list">
        <RouterLink
          v-for="link in recentLinks"
          :key="link.id"
          :to="`/links/${link.id}`"
          class="recent-row"
        >
          <span class="recent-mark" :class="{ 'is-disabled': link.status !== 1 }" />
          <span class="recent-main">
            <strong>{{ link.title || `/${link.slug}` }}</strong>
            <span>{{ shortDestination(link.destination_url) }}</span>
          </span>
          <span class="recent-code">/{{ link.slug }}</span>
          <span class="status-text">{{ statusLabel(link.status) }}</span>
          <time>{{ formatDateTime(link.created_at) }}</time>
        </RouterLink>
      </div>
    </section>

    <LinkFormDialog
      :open="createOpen"
      @close="createOpen = false"
      @saved="handleCreated"
    />
  </div>
</template>

<style scoped>
.dashboard-overview-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.45fr) minmax(18rem, 0.55fr);
  gap: 1rem;
}

.overview-trend-panel,
.overview-top-panel {
  min-width: 0;
}

.workspace-top-list {
  display: grid;
  gap: 0;
  padding: 0;
  margin: 1rem 0 0;
  list-style: none;
}

.workspace-top-list li {
  border-top: 1px solid var(--line);
}

.workspace-top-list li:first-child {
  border-top: 0;
}

.workspace-top-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  padding: 0.9rem 0;
  text-decoration: none;
}

.workspace-top-row:hover .workspace-top-title strong {
  color: var(--accent-strong);
}

.workspace-top-title {
  display: grid;
  min-width: 0;
}

.workspace-top-title strong,
.workspace-top-title span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.workspace-top-title span {
  color: var(--muted);
  font-family: var(--font-mono);
  font-size: 0.76rem;
}

.workspace-top-clicks {
  flex: 0 0 auto;
  color: var(--muted-strong);
  font: 600 0.78rem/1 var(--font-mono);
}

@media (max-width: 980px) {
  .dashboard-overview-grid {
    grid-template-columns: 1fr;
  }
}
</style>
