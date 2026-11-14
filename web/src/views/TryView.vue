<script setup lang="ts">
import {
  computed,
  onBeforeUnmount,
  onMounted,
  reactive,
  shallowRef,
  watch,
} from 'vue'
import { RouterLink } from 'vue-router'

import {
  createTrialLink,
  getTrialAnalytics,
  getTrialQRCode,
  listTrialLinks,
} from '@/api'
import { ApiError } from '@/api/http'
import type { AnalyticsOverview, TrialLink } from '@/api/types'
import TrendChart from '@/components/TrendChart.vue'
import { formatCompactNumber, formatDateTime } from '@/utils/format'

type ExpiryMinutes = 15 | 30 | 60 | 720 | 1440

const expiryOptions: ReadonlyArray<{ value: ExpiryMinutes; label: string }> = [
  { value: 15, label: '15 分钟' },
  { value: 30, label: '30 分钟' },
  { value: 60, label: '1 小时' },
  { value: 720, label: '12 小时' },
  { value: 1440, label: '24 小时' },
]

const benefits = [
  {
    title: '无需注册',
    description: '打开页面就能创建，当前浏览器自动保存试用记录。',
  },
  {
    title: '有效时间可控',
    description: '从 15 分钟到 24 小时，按分享场景选择生命周期。',
  },
  {
    title: '可继续追踪',
    description: '注册后可保留链接和点击数据，查看完整访问分析。',
  },
] as const

const steps = [
  {
    title: '粘贴目标网址',
    description: '输入需要分享的完整 HTTP 或 HTTPS 地址。',
  },
  {
    title: '选择短码和时长',
    description: '短码可选，有效期最短 15 分钟、最长 24 小时。',
  },
  {
    title: '复制并分享',
    description: '生成后复制短链，注册即可保留并分析访问数据。',
  },
] as const

const form = reactive({
  destinationURL: '',
  alias: '',
  title: '',
  expiryMinutes: 1440 as ExpiryMinutes,
})
const links = shallowRef<TrialLink[]>([])
const latestLink = shallowRef<TrialLink | null>(null)
const remaining = shallowRef(3)
const limit = shallowRef(3)
const sessionExpiresAt = shallowRef<string | null>(null)
const loading = shallowRef(true)
const pending = shallowRef(false)
const errorMessage = shallowRef('')
const notice = shallowRef('')
const selectedLink = shallowRef<TrialLink | null>(null)
const trialAnalytics = shallowRef<AnalyticsOverview | null>(null)
const insightDays = shallowRef<1 | 7>(1)
const insightLoading = shallowRef(false)
const insightError = shallowRef('')
const qrPreviewURL = shallowRef('')
const qrBlob = shallowRef<Blob | null>(null)
let insightRequestID = 0

const exhausted = computed(() => remaining.value === 0)
const quotaLabel = computed(() => `剩余 ${remaining.value} / ${limit.value} 条`)
const selectedTrend = computed(() => trialAnalytics.value?.trend ?? [])

// loadTrialLinks 读取当前浏览器 Cookie 对应的游客链接。
async function loadTrialLinks(): Promise<void> {
  try {
    const result = await listTrialLinks()
    links.value = result.items
    remaining.value = result.remaining
    limit.value = result.limit
    sessionExpiresAt.value = result.expires_at
  } catch (error) {
    errorMessage.value =
      error instanceof ApiError ? error.message : '加载试用链接失败，请稍后重试'
  } finally {
    loading.value = false
  }
}

// submit 创建一条无需登录的短期短链。
async function submit(): Promise<void> {
  if (pending.value || exhausted.value) {
    return
  }
  pending.value = true
  errorMessage.value = ''
  notice.value = ''
  try {
    const created = await createTrialLink({
      destination_url: form.destinationURL.trim(),
      alias: form.alias.trim() || undefined,
      title: form.title.trim() || undefined,
      expires_in_minutes: form.expiryMinutes,
    })
    latestLink.value = created
    form.destinationURL = ''
    form.alias = ''
    form.title = ''
    await loadTrialLinks()
    notice.value = '试用短链已生成，可以复制后立即分享。'
  } catch (error) {
    errorMessage.value =
      error instanceof ApiError ? error.message : '创建试用短链失败，请稍后重试'
  } finally {
    pending.value = false
  }
}

// copyShortURL 复制短链并在页面内给出反馈。
async function copyShortURL(value: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(value)
    notice.value = '短链已复制'
  } catch {
    notice.value = '浏览器未允许复制，请手动选择短链'
  }
}

// openInsights 展开游客短链的二维码和基础访问分析。
async function openInsights(item: TrialLink): Promise<void> {
  selectedLink.value = item
  trialAnalytics.value = null
  await loadInsights()
}

// loadInsights 并行加载当前游客短链的分析与二维码。
async function loadInsights(): Promise<void> {
  const current = selectedLink.value
  if (!current) {
    return
  }
  const requestID = ++insightRequestID
  insightLoading.value = true
  insightError.value = ''
  try {
    const [overview, qr] = await Promise.all([
      getTrialAnalytics(current.id, insightDays.value),
      getTrialQRCode(current.id),
    ])
    if (requestID !== insightRequestID || selectedLink.value?.id !== current.id) {
      return
    }
    trialAnalytics.value = overview
    replaceQRPreview(qr)
  } catch (error) {
    if (requestID === insightRequestID) {
      insightError.value =
        error instanceof ApiError ? error.message : '加载访问数据失败'
    }
  } finally {
    if (requestID === insightRequestID) {
      insightLoading.value = false
    }
  }
}

// replaceQRPreview 替换二维码对象 URL 并释放旧资源。
function replaceQRPreview(blob: Blob): void {
  if (qrPreviewURL.value) {
    URL.revokeObjectURL(qrPreviewURL.value)
  }
  qrBlob.value = blob
  qrPreviewURL.value = URL.createObjectURL(blob)
}

// closeInsights 关闭游客数据面板并释放二维码资源。
function closeInsights(): void {
  insightRequestID += 1
  selectedLink.value = null
  trialAnalytics.value = null
  if (qrPreviewURL.value) {
    URL.revokeObjectURL(qrPreviewURL.value)
    qrPreviewURL.value = ''
  }
  qrBlob.value = null
}

// downloadInsightQR 下载当前游客短链二维码。
function downloadInsightQR(): void {
  if (!qrBlob.value || !selectedLink.value) {
    return
  }
  const anchor = document.createElement('a')
  const url = URL.createObjectURL(qrBlob.value)
  anchor.href = url
  anchor.download = `${selectedLink.value.slug}.png`
  anchor.click()
  URL.revokeObjectURL(url)
}

watch(insightDays, () => {
  if (selectedLink.value) {
    void loadInsights()
  }
})

// scrollToComposer 将页面带回首屏创建区，方便继续创建下一条试用短链。
function scrollToComposer(): void {
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

onMounted(loadTrialLinks)

onBeforeUnmount(() => {
  if (qrPreviewURL.value) {
    URL.revokeObjectURL(qrPreviewURL.value)
  }
})
</script>

<template>
  <main class="trial-page">
    <header class="landing-header">
      <div class="landing-shell landing-nav">
        <RouterLink class="landing-brand" to="/try" aria-label="短链控制台首页">
          <span class="brand-mark" aria-hidden="true">SL</span>
          <span class="landing-brand-copy">
            <strong>短链控制台</strong>
            <small>Short Link Workspace</small>
          </span>
        </RouterLink>

        <nav class="landing-actions" aria-label="账号选项">
          <RouterLink class="landing-report-link" to="/report">举报违规短链</RouterLink>
          <RouterLink class="landing-login-link" to="/login">登录</RouterLink>
          <RouterLink class="landing-register-link" to="/register">免费注册</RouterLink>
        </nav>
      </div>
    </header>

    <section class="hero-section">
      <div class="landing-shell hero-content">
        <p class="hero-eyebrow">
          <span class="eyebrow-dot" aria-hidden="true" />
          无需登录 · 短期有效 · 点击数据可追踪
        </p>
        <h1>把长链接，变成一条<span class="no-break">好分享</span>的短地址。</h1>
        <p class="hero-lede">
          粘贴目标网址，按需选择短码和有效期。创建后可以立即复制，
          注册后还能保留链接并查看完整访问分析。
        </p>

        <div class="promise-row" aria-label="试用说明">
          <span><b aria-hidden="true">✓</b> 免登录创建</span>
          <span><b aria-hidden="true">✓</b> 自定义短码</span>
          <span><b aria-hidden="true">✓</b> 15 分钟至 24 小时</span>
          <span><b aria-hidden="true">✓</b> 注册后保留数据</span>
        </div>

        <section class="shorten-panel" aria-labelledby="shorten-title">
          <div class="shorten-heading">
            <div>
              <h2 id="shorten-title">创建试用短链</h2>
              <p>{{ quotaLabel }}，当前浏览器会话到期后自动失效。</p>
            </div>
            <span v-if="sessionExpiresAt" class="session-chip">
              会话至 {{ formatDateTime(sessionExpiresAt) }}
            </span>
          </div>

          <form class="shorten-form" @submit.prevent="submit">
            <label class="trial-field trial-field-target">
              <span>目标地址</span>
              <input
                v-model="form.destinationURL"
                name="destination_url"
                type="url"
                inputmode="url"
                autocomplete="url"
                placeholder="https://example.com/a-very-long-link"
                required
                maxlength="2048"
                :disabled="exhausted"
              />
            </label>

            <div class="trial-field-grid">
              <label class="trial-field">
                <span>自定义短码，可选</span>
                <input
                  v-model="form.alias"
                  name="alias"
                  type="text"
                  maxlength="64"
                  pattern="[A-Za-z0-9_\-]+"
                  placeholder="launch"
                  :disabled="exhausted"
                />
              </label>
              <label class="trial-field">
                <span>标题，可选</span>
                <input
                  v-model="form.title"
                  name="title"
                  type="text"
                  maxlength="255"
                  placeholder="活动页面"
                  :disabled="exhausted"
                />
              </label>
            </div>

            <fieldset class="expiry-fieldset" :disabled="exhausted">
              <legend>链接有效期</legend>
              <div class="expiry-options">
                <label
                  v-for="option in expiryOptions"
                  :key="option.value"
                  class="expiry-chip"
                  :class="{ 'is-selected': form.expiryMinutes === option.value }"
                >
                  <input
                    v-model="form.expiryMinutes"
                    type="radio"
                    name="expiry_minutes"
                    :value="option.value"
                  />
                  <span>{{ option.label }}</span>
                </label>
              </div>
            </fieldset>

            <p v-if="errorMessage" class="form-error" role="alert">{{ errorMessage }}</p>
            <p v-if="notice" class="inline-notice" role="status">{{ notice }}</p>

            <button class="primary-button shorten-submit" type="submit" :disabled="pending || exhausted">
              {{
                pending
                  ? '正在生成…'
                  : exhausted
                    ? '本次试用额度已用完'
                    : '生成短链接'
              }}
            </button>
            <p class="form-footnote">
              创建即表示目标地址合法且允许公开分享；系统会检查目标网址格式。
            </p>
          </form>

          <section v-if="latestLink" class="trial-result" aria-labelledby="latest-result-title">
            <div class="trial-result-main">
              <div class="result-badge-row">
                <span class="result-badge"><span aria-hidden="true" />短链接已生成</span>
                <span class="result-expiry">失效时间：{{ formatDateTime(latestLink.expires_at) }}</span>
              </div>
              <h3 id="latest-result-title">{{ latestLink.title || `/${latestLink.slug}` }}</h3>
              <a :href="latestLink.short_url" target="_blank" rel="noreferrer">
                {{ latestLink.short_url }}
              </a>
              <p>{{ latestLink.destination_url }}</p>
            </div>
            <div class="trial-result-actions">
              <button
                class="secondary-button"
                type="button"
                @click="copyShortURL(latestLink.short_url)"
              >
                复制短链
              </button>
              <button class="text-button" type="button" @click="openInsights(latestLink)">
                查看访问与二维码
              </button>
            </div>
          </section>
        </section>

        <ul class="benefit-row" aria-label="免登录试用能力">
          <li v-for="benefit in benefits" :key="benefit.title">
            <span class="benefit-mark" aria-hidden="true" />
            <div>
              <strong>{{ benefit.title }}</strong>
              <p>{{ benefit.description }}</p>
            </div>
          </li>
        </ul>
      </div>
    </section>

    <section class="landing-shell landing-section" aria-labelledby="trial-list-title">
      <div class="section-title-row">
        <div>
          <p class="section-kicker">当前试用会话</p>
          <h2 id="trial-list-title">这个浏览器创建的短链</h2>
        </div>
        <span>{{ quotaLabel }}</span>
      </div>

      <p v-if="loading" class="quiet-state" role="status">正在读取试用链接…</p>
      <div v-else-if="links.length === 0" class="trial-empty">
        <strong>还没有试用短链</strong>
        <p>在上方粘贴一个网址，生成后会出现在这里。</p>
        <button class="text-button" type="button" @click="scrollToComposer">
          回到创建区
        </button>
      </div>
      <ul v-else class="trial-link-list">
        <li v-for="item in links" :key="item.id" class="trial-link-row">
          <div class="trial-link-main">
            <strong>{{ item.title || `/${item.slug}` }}</strong>
            <span>{{ item.destination_url }}</span>
          </div>
          <div class="trial-link-meta">
            <a :href="item.short_url" target="_blank" rel="noreferrer">{{ item.short_url }}</a>
            <time>{{ formatDateTime(item.expires_at) }}</time>
          </div>
          <div class="trial-link-actions">
            <button class="text-button" type="button" @click="copyShortURL(item.short_url)">
              复制
            </button>
            <button class="text-button" type="button" @click="openInsights(item)">
              查看数据
            </button>
          </div>
        </li>
      </ul>
    </section>

    <section
      v-if="selectedLink"
      class="landing-shell trial-insights"
      aria-labelledby="trial-insights-title"
    >
      <div class="trial-insights-heading">
        <div>
          <p class="section-kicker">试用短链数据</p>
          <h2 id="trial-insights-title">{{ selectedLink.title || `/${selectedLink.slug}` }}</h2>
          <p>访客打开短链后，基础数据会自动汇总到这里。</p>
        </div>
        <div class="trial-insights-actions">
          <label class="compact-field">
            <span>范围</span>
            <select v-model="insightDays">
              <option :value="1">今天</option>
              <option :value="7">最近 7 天</option>
            </select>
          </label>
          <button class="secondary-button" type="button" @click="closeInsights">关闭</button>
        </div>
      </div>

      <p v-if="insightLoading" class="quiet-state" role="status">正在加载访问数据…</p>
      <div v-else-if="insightError" class="error-panel" role="alert">
        <p>{{ insightError }}</p>
        <button class="secondary-button" type="button" @click="loadInsights">重新加载</button>
      </div>
      <template v-else-if="trialAnalytics">
        <section class="trial-insight-stats" aria-label="短链访问概况">
          <div>
            <span>总访问</span>
            <strong>{{ formatCompactNumber(trialAnalytics.summary.total_clicks) }}</strong>
          </div>
          <div>
            <span>常规访问</span>
            <strong>{{ formatCompactNumber(trialAnalytics.summary.human_clicks) }}</strong>
          </div>
          <div>
            <span>自动访问</span>
            <strong>{{ formatCompactNumber(trialAnalytics.summary.bot_clicks) }}</strong>
          </div>
          <div>
            <span>独立访客</span>
            <strong>{{ formatCompactNumber(trialAnalytics.summary.unique_visitors) }}</strong>
          </div>
        </section>

        <div class="trial-insight-grid">
          <section class="trial-insight-trend">
            <h3>访问趋势</h3>
            <p v-if="selectedTrend.length === 0" class="quiet-state">
              还没有访问记录。打开一次短链后再回来查看。
            </p>
            <TrendChart v-else :points="selectedTrend" />
          </section>
          <aside class="trial-insight-qr">
            <h3>二维码</h3>
            <div class="trial-qr-preview">
              <img v-if="qrPreviewURL" :src="qrPreviewURL" :alt="`短链接 /${selectedLink.slug} 的二维码`" />
            </div>
            <div class="trial-qr-actions">
              <button class="secondary-button" type="button" @click="downloadInsightQR">
                下载 PNG
              </button>
              <a class="secondary-button" :href="selectedLink.short_url" target="_blank" rel="noreferrer">
                打开短链
              </a>
            </div>
          </aside>
        </div>
      </template>
    </section>

    <section class="landing-shell landing-section steps-section" aria-labelledby="steps-title">
      <div class="section-title-row section-title-row-centered">
        <div>
          <p class="section-kicker">使用方式</p>
          <h2 id="steps-title">三步生成一条可分享链接</h2>
        </div>
      </div>
      <ol class="step-list">
        <li v-for="(step, index) in steps" :key="step.title" class="step-card">
          <span class="step-number" aria-hidden="true">{{ index + 1 }}</span>
          <h3>{{ step.title }}</h3>
          <p>{{ step.description }}</p>
        </li>
      </ol>
    </section>

    <footer class="landing-footer">
      <div class="landing-shell footer-content">
        <div>
          <strong>短链控制台</strong>
          <p>创建简单，分享清楚，数据可追踪。</p>
        </div>
        <nav aria-label="页脚导航">
          <RouterLink to="/report">举报违规短链</RouterLink>
          <RouterLink to="/login">登录</RouterLink>
          <RouterLink to="/register">创建账号</RouterLink>
        </nav>
      </div>
    </footer>
  </main>
</template>

<style scoped>
.trial-page {
  min-height: 100vh;
  overflow: hidden;
  background:
    radial-gradient(circle at 50% -12rem, rgba(22, 124, 108, 0.11), transparent 34rem),
    var(--surface);
}

.landing-shell {
  width: min(1120px, calc(100% - 2rem));
  margin: 0 auto;
}

.landing-header {
  border-bottom: 1px solid rgba(185, 201, 197, 0.7);
  background: rgba(249, 251, 250, 0.9);
  backdrop-filter: blur(14px);
}

.landing-nav {
  display: flex;
  min-height: 4.8rem;
  align-items: center;
  justify-content: space-between;
  gap: 1.5rem;
}

.landing-brand {
  display: inline-flex;
  align-items: center;
  gap: 0.75rem;
  text-decoration: none;
}

.landing-brand .brand-mark {
  border-color: var(--line-strong);
  background: var(--accent-soft);
}

.landing-brand-copy {
  display: grid;
  line-height: 1.25;
}

.landing-brand-copy strong {
  font-size: 1rem;
  letter-spacing: 0.01em;
}

.landing-brand-copy small {
  color: var(--muted);
  font: 500 0.62rem/1.4 var(--font-mono);
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.landing-actions {
  display: flex;
  align-items: center;
  gap: 0.55rem;
}

.landing-report-link,
.landing-login-link,
.landing-register-link {
  padding: 0.58rem 0.85rem;
  border: 1px solid transparent;
  border-radius: var(--radius-sm);
  font-size: 0.82rem;
  font-weight: 650;
  text-decoration: none;
}

.landing-report-link {
  color: var(--muted-strong);
}

.landing-report-link:hover {
  color: var(--accent-strong);
}

.landing-login-link {
  border-color: var(--line-strong);
  background: var(--surface-strong);
  color: var(--ink);
}

.landing-register-link {
  background: var(--accent);
  color: #ffffff;
}

.landing-register-link:hover {
  background: var(--accent-strong);
}

.hero-section {
  padding: clamp(2.5rem, 6vw, 5rem) 0 3.5rem;
}

.hero-content {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
}

.hero-eyebrow {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  margin: 0;
  color: var(--accent-strong);
  font-size: 0.78rem;
  font-weight: 700;
}

.eyebrow-dot {
  width: 0.42rem;
  height: 0.42rem;
  border-radius: 50%;
  background: var(--accent);
  box-shadow: 0 0 0 0.24rem rgba(22, 124, 108, 0.12);
}

.hero-content h1 {
  max-width: 18ch;
  margin: 0.8rem 0 0;
  color: var(--ink);
  font-family: var(--font-body);
  font-size: clamp(2.4rem, 5.2vw, 4.4rem);
  font-weight: 760;
  letter-spacing: -0.045em;
  line-height: 1.04;
}

.no-break {
  white-space: nowrap;
}

.hero-lede {
  max-width: 48rem;
  margin: 1rem 0 0;
  color: var(--muted-strong);
  font-size: clamp(0.94rem, 1.5vw, 1.08rem);
  line-height: 1.75;
}

.promise-row {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 0.55rem;
  margin: 1.15rem 0 1.4rem;
}

.promise-row span {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  padding: 0.4rem 0.65rem;
  border: 1px solid var(--line);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.78);
  color: var(--muted-strong);
  font-size: 0.74rem;
}

.promise-row b {
  color: var(--accent);
  font-size: 0.72rem;
}

.shorten-panel {
  width: min(100%, 760px);
  padding: clamp(1rem, 3vw, 1.55rem);
  border: 1px solid var(--line);
  border-radius: 1.35rem;
  background: var(--surface-strong);
  box-shadow: 0 22px 55px rgba(35, 74, 68, 0.1);
  text-align: left;
}

.shorten-heading {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
  margin-bottom: 1.1rem;
}

.shorten-heading h2 {
  margin: 0;
  font-family: var(--font-body);
  font-size: 1.15rem;
  letter-spacing: -0.02em;
}

.shorten-heading p {
  margin: 0.35rem 0 0;
  color: var(--muted);
  font-size: 0.76rem;
}

.session-chip {
  flex: 0 0 auto;
  padding: 0.38rem 0.55rem;
  border-radius: var(--radius-sm);
  background: var(--accent-soft);
  color: var(--accent-strong);
  font-size: 0.7rem;
  font-weight: 650;
}

.shorten-form {
  display: grid;
  gap: 0.9rem;
}

.trial-field {
  display: grid;
  gap: 0.42rem;
  min-width: 0;
}

.trial-field > span,
.expiry-fieldset legend {
  color: var(--muted-strong);
  font-size: 0.73rem;
  font-weight: 700;
}

.trial-field input {
  width: 100%;
  min-height: 2.85rem;
  padding: 0.66rem 0.8rem;
  border: 1px solid var(--line-strong);
  border-radius: var(--radius-sm);
  background: #fbfdfc;
  color: var(--ink);
  outline: none;
}

.trial-field-target input {
  min-height: 3.2rem;
  font-family: var(--font-mono);
  font-size: 0.88rem;
}

.trial-field input:focus {
  border-color: var(--accent);
  box-shadow: 0 0 0 3px rgba(22, 124, 108, 0.12);
}

.trial-field-grid {
  display: grid;
  grid-template-columns: 1fr 1.3fr;
  gap: 0.8rem;
}

.expiry-fieldset {
  padding: 0;
  border: 0;
}

.expiry-fieldset legend {
  margin-bottom: 0.45rem;
}

.expiry-options {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: 0.42rem;
  padding: 0.35rem;
  border-radius: var(--radius-sm);
  background: var(--bg);
}

.expiry-chip {
  position: relative;
  display: grid;
  min-height: 2.4rem;
  place-items: center;
  border: 1px solid transparent;
  border-radius: calc(var(--radius-sm) - 0.12rem);
  color: var(--muted-strong);
  cursor: pointer;
  font-size: 0.74rem;
  font-weight: 650;
}

.expiry-chip input {
  position: absolute;
  inset: 0;
  opacity: 0;
  cursor: pointer;
}

.expiry-chip:hover {
  color: var(--accent-strong);
}

.expiry-chip.is-selected {
  border-color: var(--line-strong);
  background: var(--surface-strong);
  color: var(--accent-strong);
  box-shadow: 0 3px 10px rgba(30, 68, 62, 0.08);
}

.expiry-chip:has(input:focus-visible) {
  outline: 3px solid rgba(22, 124, 108, 0.24);
  outline-offset: 2px;
}

.shorten-submit {
  width: 100%;
  min-height: 3.1rem;
  justify-content: center;
  font-size: 0.94rem;
}

.form-footnote {
  margin: -0.2rem 0 0;
  color: var(--muted);
  font-size: 0.7rem;
  text-align: center;
}

.trial-result {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1.25rem;
  margin-top: 1rem;
  padding: 1rem;
  border: 1px solid #9ecdc2;
  border-radius: var(--radius-md);
  background: var(--accent-soft);
}

.trial-result-main {
  min-width: 0;
}

.result-badge-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.6rem;
}

.result-badge {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  color: var(--accent-strong);
  font-size: 0.7rem;
  font-weight: 750;
}

.result-badge span {
  width: 0.4rem;
  height: 0.4rem;
  border-radius: 50%;
  background: var(--accent);
}

.result-expiry {
  color: var(--muted);
  font-size: 0.68rem;
}

.trial-result h3 {
  margin: 0.45rem 0 0.25rem;
  font-size: 1rem;
}

.trial-result a {
  color: var(--accent-strong);
  font-family: var(--font-mono);
  font-size: 0.84rem;
  overflow-wrap: anywhere;
}

.trial-result p {
  margin: 0.35rem 0 0;
  overflow: hidden;
  color: var(--muted);
  font-size: 0.75rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.trial-result-actions,
.trial-link-actions {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 0.65rem;
}

.trial-link-actions {
  flex-direction: column;
  align-items: flex-end;
}

.trial-insights {
  display: grid;
  gap: 1rem;
  margin-top: 1rem;
  padding: clamp(1rem, 2.5vw, 1.5rem);
  border: 1px solid var(--line);
  border-radius: var(--radius-lg);
  background: var(--surface-strong);
}

.trial-insights-heading {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 1rem;
}

.trial-insights-heading h2 {
  margin: 0;
  font-family: var(--font-body);
  font-size: clamp(1.3rem, 3vw, 1.8rem);
}

.trial-insights-heading p:last-child {
  margin: 0.35rem 0 0;
  color: var(--muted);
  font-size: 0.78rem;
}

.trial-insights-actions {
  display: flex;
  align-items: flex-end;
  gap: 0.55rem;
}

.trial-insight-stats {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  border: 1px solid var(--line);
  border-radius: var(--radius-md);
  overflow: hidden;
}

.trial-insight-stats div {
  display: grid;
  gap: 0.35rem;
  padding: 0.9rem 1rem;
  border-left: 1px solid var(--line);
}

.trial-insight-stats div:first-child {
  border-left: 0;
  background: var(--rail);
  color: #ffffff;
}

.trial-insight-stats span {
  color: inherit;
  font-size: 0.7rem;
  opacity: 0.72;
}

.trial-insight-stats strong {
  font-size: 1.55rem;
  line-height: 1;
}

.trial-insight-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.45fr) minmax(13rem, 0.55fr);
  gap: 1rem;
}

.trial-insight-trend,
.trial-insight-qr {
  padding: 1rem;
  border: 1px solid var(--line);
  border-radius: var(--radius-md);
}

.trial-insight-trend h3,
.trial-insight-qr h3 {
  margin: 0 0 0.55rem;
  font-family: var(--font-body);
}

.trial-qr-preview {
  display: grid;
  min-height: 11rem;
  place-items: center;
  border-radius: var(--radius-sm);
  background: var(--bg);
}

.trial-qr-preview img {
  width: min(100%, 12rem);
  height: auto;
  display: block;
}

.trial-qr-actions {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0.5rem;
  margin-top: 0.65rem;
}

.benefit-row {
  display: grid;
  width: min(100%, 920px);
  grid-template-columns: repeat(3, 1fr);
  gap: 1rem;
  padding: 0;
  margin: 2rem 0 0;
  list-style: none;
  text-align: left;
}

.benefit-row li {
  display: flex;
  align-items: flex-start;
  gap: 0.65rem;
}

.benefit-mark {
  width: 0.55rem;
  height: 0.55rem;
  flex: 0 0 auto;
  margin-top: 0.35rem;
  transform: rotate(45deg);
  border: 2px solid var(--accent);
}

.benefit-row strong {
  font-size: 0.8rem;
}

.benefit-row p {
  margin: 0.25rem 0 0;
  color: var(--muted);
  font-size: 0.73rem;
  line-height: 1.55;
}

.landing-section {
  padding: 2.3rem 0;
}

.section-title-row {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 1rem;
  margin-bottom: 1rem;
}

.section-title-row h2 {
  margin: 0.2rem 0 0;
  font-family: var(--font-body);
  font-size: clamp(1.35rem, 3vw, 1.9rem);
  letter-spacing: -0.025em;
}

.section-title-row > span {
  color: var(--muted);
  font-size: 0.76rem;
}

.section-title-row-centered {
  justify-content: center;
  text-align: center;
}

.trial-empty {
  padding: 2rem;
  border: 1px dashed var(--line-strong);
  border-radius: var(--radius-lg);
  background: rgba(255, 255, 255, 0.62);
  text-align: center;
}

.trial-empty strong {
  font-size: 1rem;
}

.trial-empty p {
  margin: 0.35rem 0 0.8rem;
  color: var(--muted);
  font-size: 0.82rem;
}

.trial-link-list {
  display: grid;
  gap: 0;
  padding: 0 1rem;
  margin: 0;
  border: 1px solid var(--line);
  border-radius: var(--radius-lg);
  background: var(--surface-strong);
  list-style: none;
}

.trial-link-row {
  display: grid;
  grid-template-columns: minmax(0, 1.2fr) minmax(12rem, 0.8fr) auto;
  gap: 1rem;
  align-items: center;
  padding: 1rem 0;
  border-top: 1px solid var(--line);
}

.trial-link-row:first-child {
  border-top: 0;
}

.trial-link-main {
  display: grid;
  min-width: 0;
}

.trial-link-main strong,
.trial-link-main span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.trial-link-main span {
  color: var(--muted);
  font-size: 0.75rem;
}

.trial-link-meta {
  display: grid;
  min-width: 0;
}

.trial-link-meta a {
  overflow: hidden;
  color: var(--accent-strong);
  font: 600 0.74rem/1.4 var(--font-mono);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.trial-link-meta time {
  color: var(--muted);
  font-size: 0.7rem;
}

.steps-section {
  padding-top: 3.4rem;
}

.step-list {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 1rem;
  padding: 0;
  margin: 1.4rem 0 0;
  list-style: none;
  counter-reset: none;
}

.step-card {
  min-height: 10rem;
  padding: 1.25rem;
  border: 1px solid var(--line);
  border-radius: var(--radius-lg);
  background: rgba(255, 255, 255, 0.7);
}

.step-number {
  display: inline-grid;
  width: 1.75rem;
  height: 1.75rem;
  place-items: center;
  border-radius: 50%;
  background: var(--accent-soft);
  color: var(--accent-strong);
  font: 700 0.72rem/1 var(--font-mono);
}

.step-card h3 {
  margin: 0.8rem 0 0;
  font-family: var(--font-body);
  font-size: 0.94rem;
}

.step-card p {
  margin: 0.35rem 0 0;
  color: var(--muted);
  font-size: 0.76rem;
  line-height: 1.6;
}

.landing-footer {
  margin-top: 1.5rem;
  border-top: 1px solid var(--line);
  background: #eef4f2;
}

.footer-content {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 2rem;
  min-height: 7rem;
}

.footer-content strong {
  font-size: 0.88rem;
}

.footer-content p {
  margin: 0.25rem 0 0;
  color: var(--muted);
  font-size: 0.74rem;
}

.footer-content nav {
  display: flex;
  flex-wrap: wrap;
  gap: 1rem;
}

.footer-content a {
  color: var(--muted-strong);
  font-size: 0.74rem;
  text-decoration: none;
}

.footer-content a:hover {
  color: var(--accent-strong);
}

@media (max-width: 760px) {
  .landing-shell {
    width: min(100% - 1.25rem, 1120px);
  }

  .landing-nav {
    min-height: 4.2rem;
  }

  .landing-report-link {
    display: none;
  }

  .landing-brand-copy small {
    display: none;
  }

  .hero-section {
    padding-top: 2rem;
  }

  .hero-content h1 {
    max-width: 15ch;
    font-size: clamp(2.1rem, 9vw, 3rem);
  }

  .hero-lede {
    max-width: 35rem;
    font-size: 0.9rem;
  }

  .shorten-panel {
    border-radius: var(--radius-lg);
  }

  .trial-field-grid,
  .benefit-row,
  .step-list {
    grid-template-columns: 1fr;
  }

  .expiry-options {
    grid-template-columns: repeat(2, 1fr);
  }

  .expiry-chip:last-child {
    grid-column: 1 / -1;
  }

  .benefit-row {
    gap: 0.8rem;
  }

  .trial-link-row {
    grid-template-columns: minmax(0, 1fr) auto;
  }

  .trial-link-meta {
    grid-column: 1 / -1;
    grid-row: 2;
  }

  .trial-insights-heading {
    align-items: flex-start;
    flex-direction: column;
  }

  .trial-insight-stats {
    grid-template-columns: 1fr 1fr;
  }

  .trial-insight-stats div:nth-child(3) {
    border-left: 0;
    border-top: 1px solid var(--line);
  }

  .trial-insight-stats div:nth-child(4) {
    border-top: 1px solid var(--line);
  }

  .trial-insight-grid {
    grid-template-columns: 1fr;
  }

  .footer-content {
    align-items: flex-start;
    flex-direction: column;
    justify-content: center;
    gap: 0.8rem;
  }
}

@media (max-width: 520px) {
  .landing-actions {
    gap: 0.35rem;
  }

  .landing-login-link,
  .landing-register-link {
    padding: 0.52rem 0.62rem;
    font-size: 0.76rem;
  }

  .shorten-heading,
  .trial-result {
    align-items: stretch;
    flex-direction: column;
  }

  .session-chip {
    align-self: flex-start;
  }

  .trial-result-actions {
    width: 100%;
    align-items: stretch;
    flex-direction: column;
  }

  .trial-insights-actions {
    align-items: stretch;
    flex-direction: column;
    width: 100%;
  }

  .section-title-row {
    align-items: flex-start;
    flex-direction: column;
  }

  .trial-link-list {
    padding: 0 0.85rem;
  }

  .trial-link-row {
    grid-template-columns: 1fr;
  }

  .trial-link-meta {
    grid-column: 1;
    grid-row: auto;
  }

  .trial-link-row > .text-button {
    justify-self: start;
  }

  .trial-link-actions {
    align-items: flex-start;
    flex-direction: row;
  }
}
</style>
