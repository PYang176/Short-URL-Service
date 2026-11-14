<script setup lang="ts">
import { shallowRef, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'

import { getPublicShareReport } from '@/api'
import { ApiError } from '@/api/http'
import type { PublicShareReport } from '@/api/types'
import BreakdownList from '@/components/BreakdownList.vue'
import TrendChart from '@/components/TrendChart.vue'
import { formatCompactNumber, formatDateTime } from '@/utils/format'

const route = useRoute()
const report = shallowRef<PublicShareReport | null>(null)
const loading = shallowRef(true)
const errorMessage = shallowRef('')

// loadReport 使用公开 Token 加载脱敏访问报告
async function loadReport(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    report.value = await getPublicShareReport(String(route.params.token))
  } catch (error) {
    report.value = null
    errorMessage.value =
      error instanceof ApiError ? error.message : '公开报告不存在或已失效'
  } finally {
    loading.value = false
  }
}

watch(() => route.params.token, () => void loadReport(), { immediate: true })
</script>

<template>
  <main class="public-report-page">
    <header class="public-report-header">
      <div class="public-report-shell public-report-nav">
        <RouterLink class="public-report-brand" to="/try">
          <span class="brand-mark" aria-hidden="true">SL</span>
          <span>
            <strong>短链控制台</strong>
            <small>公开访问报告</small>
          </span>
        </RouterLink>
        <RouterLink class="public-report-cta" to="/try">创建自己的短链</RouterLink>
      </div>
    </header>

    <div class="public-report-shell public-report-body">
      <div v-if="loading" class="skeleton-stack" role="status" aria-label="正在加载公开报告">
        <span class="skeleton-hero" />
        <span class="skeleton-row" />
        <span class="skeleton-row" />
      </div>

      <section v-else-if="errorMessage" class="public-report-error" role="alert">
        <h1>报告无法访问</h1>
        <p>{{ errorMessage }}</p>
        <RouterLink class="primary-button" to="/try">返回短链工具</RouterLink>
      </section>

      <template v-else-if="report">
        <header class="public-report-title">
          <div>
            <p class="section-kicker">只读访问报告</p>
            <h1>{{ report.title }}</h1>
            <a :href="report.short_url" target="_blank" rel="noreferrer">
              {{ report.short_url }}
            </a>
          </div>
          <div class="public-report-meta">
            <span>{{ report.days }} 天范围</span>
            <span>有效至 {{ formatDateTime(report.expires_at) }}</span>
          </div>
        </header>

        <section class="public-report-stats" aria-label="访问概况">
          <div>
            <span>总访问</span>
            <strong>{{ formatCompactNumber(report.analytics.summary.total_clicks) }}</strong>
          </div>
          <div>
            <span>常规访问</span>
            <strong>{{ formatCompactNumber(report.analytics.summary.human_clicks) }}</strong>
          </div>
          <div>
            <span>自动访问</span>
            <strong>{{ formatCompactNumber(report.analytics.summary.bot_clicks) }}</strong>
          </div>
          <div>
            <span>独立访客</span>
            <strong>{{ formatCompactNumber(report.analytics.summary.unique_visitors) }}</strong>
          </div>
        </section>

        <section class="panel public-report-trend">
          <div class="section-heading-row">
            <div>
              <h2>访问趋势</h2>
              <p>报告数据由公开短链的实际访问汇总生成。</p>
            </div>
          </div>
          <p v-if="report.analytics.trend.length === 0" class="quiet-state">
            当前范围内还没有访问记录。
          </p>
          <TrendChart v-else :points="report.analytics.trend" />
        </section>

        <section class="public-breakdown-grid" aria-label="访问维度分布">
          <BreakdownList title="设备" :items="report.analytics.devices" />
          <BreakdownList title="浏览器" :items="report.analytics.browsers" />
          <BreakdownList title="操作系统" :items="report.analytics.operating_systems" />
          <BreakdownList title="国家或地区" :items="report.analytics.countries" />
        </section>

        <footer class="public-report-footer">
          <p>这是只读报告，不包含访客身份、原始 IP 或内部工作空间信息。</p>
          <RouterLink to="/try">创建短链并查看自己的访问数据</RouterLink>
        </footer>
      </template>
    </div>
  </main>
</template>

<style scoped>
.public-report-page {
  min-height: 100vh;
  background:
    radial-gradient(circle at 70% -10rem, rgba(22, 124, 108, 0.09), transparent 32rem),
    var(--surface);
}

.public-report-shell {
  width: min(76rem, calc(100% - 2rem));
  margin: 0 auto;
}

.public-report-header {
  border-bottom: 1px solid var(--line);
  background: rgba(255, 255, 255, 0.86);
  backdrop-filter: blur(14px);
}

.public-report-nav {
  display: flex;
  min-height: 4.5rem;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
}

.public-report-brand {
  display: inline-flex;
  align-items: center;
  gap: 0.65rem;
  text-decoration: none;
}

.public-report-brand .brand-mark {
  border-color: var(--line-strong);
  background: var(--accent-soft);
}

.public-report-brand > span:last-child {
  display: grid;
}

.public-report-brand small {
  color: var(--muted);
  font-size: 0.68rem;
}

.public-report-cta {
  padding: 0.52rem 0.72rem;
  border-radius: var(--radius-sm);
  background: var(--accent);
  color: #ffffff;
  font-size: 0.76rem;
  font-weight: 750;
  text-decoration: none;
}

.public-report-body {
  display: grid;
  gap: 1.2rem;
  padding: 3rem 0 4rem;
}

.public-report-title {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 1.5rem;
}

.public-report-title h1 {
  margin: 0;
  font-family: var(--font-body);
  font-size: clamp(2rem, 4vw, 3rem);
  font-weight: 780;
  letter-spacing: -0.045em;
}

.public-report-title a {
  display: inline-block;
  margin-top: 0.45rem;
  color: var(--accent-strong);
  font-family: var(--font-mono);
  font-size: 0.8rem;
  text-decoration: none;
}

.public-report-meta {
  display: grid;
  justify-items: end;
  color: var(--muted);
  font-size: 0.72rem;
}

.public-report-stats {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  overflow: hidden;
  border: 1px solid var(--line);
  border-radius: var(--radius-lg);
  background: var(--surface-strong);
}

.public-report-stats div {
  display: grid;
  gap: 0.45rem;
  padding: 1rem 1.2rem;
  border-left: 1px solid var(--line);
}

.public-report-stats div:first-child {
  border-left: 0;
  background: var(--rail);
  color: #ffffff;
}

.public-report-stats span {
  font-size: 0.72rem;
  opacity: 0.72;
}

.public-report-stats strong {
  font-size: 2rem;
  line-height: 1;
}

.public-report-trend p {
  color: var(--muted);
  font-size: 0.8rem;
}

.public-breakdown-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 1rem;
}

.public-report-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  padding-top: 1rem;
  border-top: 1px solid var(--line);
  color: var(--muted);
  font-size: 0.75rem;
}

.public-report-footer p {
  margin: 0;
}

.public-report-footer a {
  flex: 0 0 auto;
  color: var(--accent-strong);
  font-weight: 700;
}

.public-report-error {
  display: grid;
  justify-items: center;
  padding: 5rem 1rem;
  text-align: center;
}

.public-report-error h1 {
  font-family: var(--font-body);
}

.public-report-error p {
  color: var(--muted);
}

.public-report-error a {
  text-decoration: none;
}

@media (max-width: 760px) {
  .public-report-shell {
    width: calc(100% - 1.25rem);
  }

  .public-report-title {
    align-items: flex-start;
    flex-direction: column;
  }

  .public-report-meta {
    justify-items: start;
  }

  .public-report-stats,
  .public-breakdown-grid {
    grid-template-columns: 1fr 1fr;
  }

  .public-report-stats div:nth-child(3) {
    border-left: 0;
    border-top: 1px solid var(--line);
  }

  .public-report-stats div:nth-child(4) {
    border-top: 1px solid var(--line);
  }

  .public-report-footer {
    align-items: flex-start;
    flex-direction: column;
  }
}

@media (max-width: 480px) {
  .public-report-body {
    padding-top: 2rem;
  }

  .public-report-stats,
  .public-breakdown-grid {
    grid-template-columns: 1fr;
  }

  .public-report-stats div {
    border-top: 1px solid var(--line);
    border-left: 0;
  }

  .public-report-stats div:first-child {
    border-top: 0;
  }
}
</style>
