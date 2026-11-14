<script setup lang="ts">
import { computed, nextTick, onMounted, shallowRef, useTemplateRef, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { RouterLink, useRoute, useRouter } from 'vue-router'

import { getWorkspaceAnalytics } from '@/api'
import { ApiError } from '@/api/http'
import type { Link, WorkspaceAnalyticsOverview } from '@/api/types'
import AnalyticsExportPanel from '@/components/AnalyticsExportPanel.vue'
import QuickCreatePanel from '@/components/QuickCreatePanel.vue'
import TrendChart from '@/components/TrendChart.vue'
import { useLinks } from '@/composables/useLinks'
import { useAuthStore } from '@/stores/auth'
import { formatCompactNumber, formatDateTime, shortDestination, statusLabel } from '@/utils/format'

const auth = useAuthStore()
const { user } = storeToRefs(auth)
const route = useRoute()
const router = useRouter()
const quickCreate = useTemplateRef<InstanceType<typeof QuickCreatePanel>>('quickCreate')
const { links, loading, errorMessage, load } = useLinks(8)
const claimNotice = shallowRef(auth.consumeTrialClaimNotice())
const overview = shallowRef<WorkspaceAnalyticsOverview | null>(null)
const overviewDays = shallowRef(7)
const overviewLoading = shallowRef(true)
const overviewError = shallowRef('')
const recentLinks = computed(() => links.value.slice(0, 5))
const topLinks = computed(() => overview.value?.top_links ?? [])
const trendPoints = computed(() => overview.value?.trend ?? [])

onMounted(load)

// loadOverview 加载工作空间整体指标、趋势和热门链接。
async function loadOverview(): Promise<void> {
  overviewLoading.value = true
  overviewError.value = ''
  try {
    overview.value = await getWorkspaceAnalytics(overviewDays.value)
  } catch (error) {
    overviewError.value =
      error instanceof ApiError ? error.message : '加载访问分析失败'
  } finally {
    overviewLoading.value = false
  }
}

watch(overviewDays, () => void loadOverview(), { immediate: true })

// handleCreated 创建短链后刷新列表和整体分析。
async function handleCreated(_link: Link): Promise<void> {
  await Promise.all([load(), loadOverview()])
}

// focusCreate 聚焦并滚动到快速创建表单。
function focusCreate(): void {
  void quickCreate.value?.focusDestination()
}

// linkDisplayName 返回链接的自定义标题或简化后的目标地址。
function linkDisplayName(link: Link): string {
  return link.title || shortDestination(link.destination_url)
}

// handleCreateIntent 响应顶部导航的新建短链接按钮。
async function handleCreateIntent(value: unknown): Promise<void> {
  if (value !== '1') {
    return
  }
  await nextTick()
  await quickCreate.value?.focusDestination()
  await router.replace({
    query: {
      ...route.query,
      create: undefined,
    },
  })
}

watch(() => route.query.create, (value) => void handleCreateIntent(value), {
  immediate: true,
  flush: 'post',
})
</script>

<template>
  <div class="page-stack dashboard-page">
    <header class="dashboard-header">
      <div>
        <p class="section-kicker">欢迎回来</p>
        <h1 class="dashboard-title">用户控制台</h1>
        <p class="dashboard-subtitle">
          {{ user?.display_name }}，管理你的短链、二维码和访问趋势。
        </p>
      </div>
      <button class="primary-button dashboard-create-button" type="button" @click="focusCreate">
        <span aria-hidden="true">+</span>
        新建短链接
      </button>
    </header>

    <p v-if="claimNotice" class="inline-notice" role="status">{{ claimNotice }}</p>

    <section class="dashboard-stats" aria-label="工作空间统计">
      <article class="dashboard-stat-card">
        <span>短链总数</span>
        <strong>{{ overviewLoading ? '—' : (overview?.summary.total_links ?? 0) }}</strong>
        <small>{{ overview?.summary.active_links ?? 0 }} 条当前可用</small>
      </article>
      <article class="dashboard-stat-card">
        <span>{{ overviewDays }} 天累计访问</span>
        <strong>
          {{ overviewLoading ? '—' : formatCompactNumber(overview?.summary.total_clicks ?? 0) }}
        </strong>
        <small>包含自动访问</small>
      </article>
      <article class="dashboard-stat-card">
        <span>独立访客</span>
        <strong>
          {{
            overviewLoading
              ? '—'
              : formatCompactNumber(overview?.summary.unique_visitors ?? 0)
          }}
        </strong>
        <small>按每日访客哈希去重</small>
      </article>
    </section>

    <QuickCreatePanel ref="quickCreate" @created="handleCreated" />

    <section class="panel dashboard-links-panel" aria-labelledby="dashboard-links-title">
      <div class="section-heading-row">
        <div>
          <h2 id="dashboard-links-title">
            我的短链接
            <span class="dashboard-count-badge">{{ overview?.summary.total_links ?? links.length }}</span>
          </h2>
          <p>最近创建的链接会优先显示在这里。</p>
        </div>
        <RouterLink class="text-link" to="/links">查看全部</RouterLink>
      </div>

      <div v-if="loading" class="skeleton-stack" role="status" aria-label="正在加载短链接">
        <span v-for="index in 4" :key="index" class="skeleton-row" />
      </div>
      <div v-else-if="errorMessage" class="error-panel" role="alert">
        <p>{{ errorMessage }}</p>
        <button class="secondary-button" type="button" @click="load()">重新加载</button>
      </div>
      <div v-else-if="recentLinks.length === 0" class="dashboard-empty-state">
        <span class="empty-link-mark" aria-hidden="true">↗</span>
        <h3>暂无创建的短链接</h3>
        <p>在上方粘贴一个目标网址，第一条短链会立即出现在这里。</p>
        <button class="primary-button" type="button" @click="focusCreate">新建短链接</button>
      </div>
      <div v-else class="dashboard-link-list">
        <RouterLink
          v-for="link in recentLinks"
          :key="link.id"
          class="dashboard-link-row"
          :to="`/links/${link.id}`"
        >
          <span class="dashboard-link-status" :class="{ 'is-disabled': link.status !== 1 }">
            {{ statusLabel(link.status) }}
          </span>
          <span class="dashboard-link-main">
            <strong>{{ linkDisplayName(link) }}</strong>
            <small>{{ shortDestination(link.destination_url) }}</small>
          </span>
          <code>/{{ link.slug }}</code>
          <time>{{ formatDateTime(link.created_at) }}</time>
        </RouterLink>
      </div>
    </section>

    <section class="dashboard-analytics" aria-labelledby="dashboard-analytics-title">
      <div class="dashboard-section-heading">
        <div>
          <p class="section-kicker">数据分析</p>
          <h2 id="dashboard-analytics-title">访问概览</h2>
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

      <div class="dashboard-overview-grid">
        <section class="panel overview-trend-panel">
          <div class="section-heading-row">
            <div>
              <h3>整体访问趋势</h3>
              <p>聚合当前工作空间的全部短链访问。</p>
            </div>
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
            当前范围内还没有访问记录。
          </p>
          <TrendChart v-else :points="trendPoints" />
        </section>

        <section class="panel overview-top-panel">
          <div class="section-heading-row">
            <div>
              <h3>热门短链</h3>
              <p>按当前时间范围内的访问量排序。</p>
            </div>
          </div>

          <p v-if="overviewLoading" class="quiet-state" role="status">正在统计…</p>
          <div v-else-if="overviewError" class="quiet-state">
            暂时无法加载热门短链。
          </div>
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
    </section>

    <AnalyticsExportPanel />
  </div>
</template>

<style scoped>
.dashboard-page {
  gap: 1.25rem;
}

.dashboard-header {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 1.5rem;
  padding: 0.6rem 0 0.35rem;
}

.dashboard-title {
  margin: 0;
  font-family: var(--font-body);
  font-size: clamp(2rem, 4vw, 3rem);
  font-weight: 780;
  letter-spacing: -0.045em;
}

.dashboard-subtitle {
  margin: 0.45rem 0 0;
  color: var(--muted);
  font-size: 0.86rem;
}

.dashboard-create-button {
  gap: 0.35rem;
}

.dashboard-stats {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 1rem;
}

.dashboard-stat-card {
  display: grid;
  min-height: 8.4rem;
  align-content: start;
  padding: 1.15rem 1.25rem;
  border: 1px solid var(--line);
  border-radius: var(--radius-lg);
  background: var(--surface-strong);
}

.dashboard-stat-card > span {
  color: var(--muted-strong);
  font-size: 0.76rem;
  font-weight: 650;
}

.dashboard-stat-card strong {
  margin-top: 0.55rem;
  font-family: var(--font-body);
  font-size: clamp(2rem, 4vw, 2.8rem);
  font-weight: 780;
  letter-spacing: -0.04em;
  line-height: 1;
}

.dashboard-stat-card small {
  align-self: end;
  margin-top: 0.6rem;
  color: var(--muted);
  font-size: 0.7rem;
}

.dashboard-links-panel {
  padding-top: 1.15rem;
}

.dashboard-links-panel h2 {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-family: var(--font-body);
  font-size: 1.12rem;
}

.dashboard-links-panel h2 p {
  margin: 0;
}

.dashboard-links-panel .text-link {
  flex: 0 0 auto;
  white-space: nowrap;
}

.dashboard-count-badge {
  display: inline-grid;
  min-width: 1.45rem;
  height: 1.45rem;
  place-items: center;
  padding: 0 0.35rem;
  border-radius: 999px;
  background: var(--accent-soft);
  color: var(--accent-strong);
  font-size: 0.7rem;
}

.dashboard-empty-state {
  display: grid;
  justify-items: center;
  padding: 3rem 1rem 3.4rem;
  text-align: center;
}

.empty-link-mark {
  display: grid;
  width: 2.8rem;
  height: 2.8rem;
  place-items: center;
  border-radius: 50%;
  background: var(--accent-soft);
  color: var(--accent-strong);
  font-size: 1.2rem;
  font-weight: 800;
}

.dashboard-empty-state h3 {
  margin: 0.8rem 0 0;
}

.dashboard-empty-state p {
  margin: 0.35rem 0 1rem;
  color: var(--muted);
  font-size: 0.8rem;
}

.dashboard-link-list {
  display: grid;
  margin-top: 0.8rem;
}

.dashboard-link-row {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) minmax(8rem, auto) auto;
  gap: 1rem;
  align-items: center;
  padding: 0.9rem 0;
  border-top: 1px solid var(--line);
  text-decoration: none;
}

.dashboard-link-row:first-child {
  border-top: 0;
}

.dashboard-link-row:hover .dashboard-link-main strong {
  color: var(--accent-strong);
}

.dashboard-link-status {
  padding: 0.28rem 0.5rem;
  border-radius: 999px;
  background: var(--accent-soft);
  color: var(--accent-strong);
  font-size: 0.68rem;
  font-weight: 700;
}

.dashboard-link-status.is-disabled {
  background: var(--danger-soft);
  color: var(--danger);
}

.dashboard-link-main {
  display: grid;
  min-width: 0;
}

.dashboard-link-main strong,
.dashboard-link-main small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dashboard-link-main small {
  color: var(--muted);
  font-size: 0.73rem;
}

.dashboard-link-row code {
  color: var(--muted-strong);
  font-family: var(--font-mono);
  font-size: 0.75rem;
}

.dashboard-link-row time {
  color: var(--muted);
  font-size: 0.7rem;
}

.dashboard-analytics {
  display: grid;
  gap: 0.8rem;
  padding-top: 1.2rem;
}

.dashboard-section-heading {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 1rem;
}

.dashboard-section-heading h2 {
  font-family: var(--font-body);
  font-size: 1.25rem;
}

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
  padding: 0;
  margin: 0.8rem 0 0;
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
  padding: 0.8rem 0;
  text-decoration: none;
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
  font-size: 0.72rem;
}

.workspace-top-clicks {
  flex: 0 0 auto;
  color: var(--muted-strong);
  font: 600 0.74rem/1 var(--font-mono);
}

@media (max-width: 980px) {
  .dashboard-overview-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 760px) {
  .dashboard-header {
    align-items: stretch;
    flex-direction: column;
  }

  .dashboard-create-button {
    width: 100%;
  }

  .dashboard-stats {
    grid-template-columns: 1fr;
  }

  .dashboard-stat-card {
    min-height: 0;
  }

  .dashboard-link-row {
    grid-template-columns: auto minmax(0, 1fr);
  }

  .dashboard-link-row code,
  .dashboard-link-row time {
    grid-column: 2;
  }

  .dashboard-section-heading {
    align-items: stretch;
    flex-direction: column;
  }
}

@media (max-width: 520px) {
  .dashboard-links-panel .section-heading-row {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    align-items: start;
  }

  .dashboard-links-panel .section-heading-row > div {
    min-width: 0;
  }
}
</style>
