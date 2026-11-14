<script setup lang="ts">
import { computed, onMounted, reactive, shallowRef } from 'vue'
import { RouterLink } from 'vue-router'

import { createTrialLink, listTrialLinks } from '@/api'
import { ApiError } from '@/api/http'
import type { TrialLink } from '@/api/types'
import { formatDateTime } from '@/utils/format'

const form = reactive({
  destinationURL: '',
  alias: '',
  title: '',
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

const exhausted = computed(() => remaining.value === 0)
const quotaLabel = computed(() => `剩余 ${remaining.value} / ${limit.value} 条`)

// loadTrialLinks 读取当前浏览器 Cookie 对应的游客链接
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

// submit 创建一条无需登录的短期短链
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
    })
    latestLink.value = created
    form.destinationURL = ''
    form.alias = ''
    form.title = ''
    await loadTrialLinks()
    notice.value = '试用短链已生成，可在当前浏览器中继续查看。'
  } catch (error) {
    errorMessage.value =
      error instanceof ApiError ? error.message : '创建试用短链失败，请稍后重试'
  } finally {
    pending.value = false
  }
}

// copyShortURL 复制最近创建的短链
async function copyShortURL(value: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(value)
    notice.value = '短链已复制'
  } catch {
    notice.value = '浏览器未允许复制，请手动选择短链'
  }
}

onMounted(loadTrialLinks)
</script>

<template>
  <main class="trial-page">
    <section class="trial-story" aria-labelledby="trial-title">
      <div class="trial-topbar">
        <div class="trial-brand">
          <span class="brand-mark" aria-hidden="true">SL</span>
          <span>短链控制台</span>
        </div>
        <nav class="trial-session-actions" aria-label="账号选项">
          <RouterLink class="trial-login-link" to="/login">登录</RouterLink>
          <RouterLink class="trial-register-link" to="/register">注册</RouterLink>
        </nav>
      </div>

      <div class="trial-copy">
        <p class="section-kicker">免登录试用</p>
        <h1 id="trial-title">先做一条能用的短链。</h1>
        <p>
          不需要注册。当前浏览器会获得一个独立游客会话，最多创建 3 条
          24 小时后自动失效的短链。
        </p>
      </div>

      <dl class="trial-facts">
        <div>
          <dt>额度</dt>
          <dd>{{ quotaLabel }}</dd>
        </div>
        <div>
          <dt>域名</dt>
          <dd>平台默认域名</dd>
        </div>
        <div>
          <dt>会话到期</dt>
          <dd>{{ sessionExpiresAt ? formatDateTime(sessionExpiresAt) : '创建后开始计时' }}</dd>
        </div>
      </dl>
    </section>

    <section class="trial-workspace" aria-label="试用短链工作区">
      <div class="trial-heading">
        <div>
          <p class="section-kicker">即时创建</p>
          <h2>生成短期短链</h2>
        </div>
        <RouterLink class="text-link" to="/login">登录使用完整功能</RouterLink>
      </div>

      <form class="trial-form" @submit.prevent="submit">
        <label class="field">
          <span>目标地址</span>
          <input
            v-model="form.destinationURL"
            name="destination_url"
            type="url"
            inputmode="url"
            placeholder="https://example.com/page"
            required
            :disabled="exhausted"
            autofocus
          />
        </label>

        <div class="trial-form-row">
          <label class="field">
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

          <label class="field">
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

        <p v-if="errorMessage" class="form-error" role="alert">{{ errorMessage }}</p>
        <p v-if="notice" class="inline-notice" role="status">{{ notice }}</p>

        <button class="primary-button" type="submit" :disabled="pending || exhausted">
          {{ pending ? '正在生成…' : exhausted ? '本次试用额度已用完' : '生成试用短链' }}
        </button>
      </form>

      <section v-if="latestLink" class="trial-result" aria-labelledby="latest-result-title">
        <div>
          <p class="section-kicker">刚刚创建</p>
          <h3 id="latest-result-title">{{ latestLink.title || `/${latestLink.slug}` }}</h3>
          <a :href="latestLink.short_url" target="_blank" rel="noreferrer">
            {{ latestLink.short_url }}
          </a>
          <p>失效时间：{{ formatDateTime(latestLink.expires_at) }}</p>
        </div>
        <button
          class="secondary-button"
          type="button"
          @click="copyShortURL(latestLink.short_url)"
        >
          复制短链
        </button>
      </section>

      <section class="trial-list-section" aria-labelledby="trial-list-title">
        <div class="trial-list-heading">
          <h3 id="trial-list-title">当前试用会话</h3>
          <span>{{ quotaLabel }}</span>
        </div>

        <p v-if="loading" class="quiet-state" role="status">正在读取试用链接…</p>
        <div v-else-if="links.length === 0" class="trial-empty">
          <p>还没有试用短链。提交一个目标地址即可开始。</p>
        </div>
        <ul v-else class="trial-link-list">
          <li v-for="item in links" :key="item.id" class="trial-link-row">
            <div>
              <strong>{{ item.title || `/${item.slug}` }}</strong>
              <span>{{ item.destination_url }}</span>
            </div>
            <div class="trial-link-meta">
              <a :href="item.short_url" target="_blank" rel="noreferrer">
                {{ item.short_url }}
              </a>
              <time>{{ formatDateTime(item.expires_at) }}</time>
            </div>
            <button
              class="text-button"
              type="button"
              @click="copyShortURL(item.short_url)"
            >
              复制
            </button>
          </li>
        </ul>
      </section>

      <p class="trial-register-note">
        注册后可进入完整工作台，管理长期链接、二维码和点击分析。
        <RouterLink to="/register">创建账号</RouterLink>
        ·
        <RouterLink to="/report">举报违规短链</RouterLink>
      </p>
    </section>
  </main>
</template>

<style scoped>
.trial-page {
  display: grid;
  grid-template-columns: minmax(20rem, 0.76fr) minmax(0, 1.24fr);
  min-height: 100vh;
}

.trial-story {
  display: flex;
  flex-direction: column;
  gap: 3rem;
  justify-content: space-between;
  padding: clamp(1.6rem, 5vw, 4rem);
  background: var(--rail);
  color: #f7fbfa;
}

.trial-brand {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  font-weight: 700;
}

.trial-topbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
}

.trial-session-actions {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.trial-login-link,
.trial-register-link {
  padding: 0.48rem 0.72rem;
  border: 1px solid rgba(255, 255, 255, 0.28);
  border-radius: var(--radius-sm);
  color: #eff8f6;
  font-size: 0.82rem;
  font-weight: 700;
  text-decoration: none;
}

.trial-register-link {
  border-color: #8fe3cf;
  background: #8fe3cf;
  color: var(--rail-deep);
}

.trial-copy {
  max-width: 34rem;
}

.trial-copy .section-kicker {
  color: #8fe3cf;
}

.trial-copy h1 {
  max-width: 10ch;
  font-size: clamp(2.8rem, 6vw, 5.2rem);
}

.trial-copy > p:last-child {
  max-width: 42ch;
  color: #bdd4d0;
}

.trial-facts {
  display: grid;
  gap: 0.8rem;
  margin: 0;
}

.trial-facts div {
  display: grid;
  grid-template-columns: 5.5rem 1fr;
  gap: 1rem;
  padding-top: 0.8rem;
  border-top: 1px solid rgba(255, 255, 255, 0.16);
}

.trial-facts dt {
  color: #8fe3cf;
  font: 600 0.72rem/1.7 var(--font-mono);
  text-transform: uppercase;
}

.trial-facts dd {
  margin: 0;
  color: #dcebe8;
}

.trial-workspace {
  display: grid;
  align-content: start;
  gap: 1.25rem;
  padding: clamp(1.5rem, 5vw, 3.5rem);
}

.trial-heading,
.trial-list-heading {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
}

.trial-heading h2 {
  margin: 0;
  font-size: clamp(1.8rem, 3vw, 2.7rem);
}

.trial-form,
.trial-result,
.trial-list-section {
  padding: clamp(1rem, 2.4vw, 1.5rem);
  border: 1px solid var(--line);
  border-radius: var(--radius-lg);
  background: var(--surface-strong);
}

.trial-form {
  display: grid;
  gap: 1rem;
}

.trial-form-row {
  display: grid;
  grid-template-columns: 1fr 1.4fr;
  gap: 1rem;
}

.trial-form .primary-button {
  justify-self: start;
}

.trial-result {
  display: flex;
  align-items: end;
  justify-content: space-between;
  gap: 1.5rem;
  border-color: #9ecdc2;
  background: var(--accent-soft);
}

.trial-result h3 {
  margin-bottom: 0.45rem;
}

.trial-result a,
.trial-link-meta a {
  overflow-wrap: anywhere;
  color: var(--accent-strong);
  font-family: var(--font-mono);
  font-size: 0.84rem;
}

.trial-result p:last-child {
  margin: 0.65rem 0 0;
  color: var(--muted-strong);
  font-size: 0.8rem;
}

.trial-list-section {
  display: grid;
  gap: 1rem;
}

.trial-list-heading h3 {
  margin: 0;
}

.trial-list-heading span {
  color: var(--muted);
  font-size: 0.8rem;
}

.trial-empty {
  padding: 1.4rem;
  border: 1px dashed var(--line-strong);
  border-radius: var(--radius-md);
  color: var(--muted);
}

.trial-link-list {
  display: grid;
  gap: 0;
  padding: 0;
  margin: 0;
  list-style: none;
}

.trial-link-row {
  display: grid;
  grid-template-columns: minmax(0, 1.25fr) minmax(11rem, 0.75fr) auto;
  gap: 1rem;
  align-items: center;
  padding: 1rem 0;
  border-top: 1px solid var(--line);
}

.trial-link-row:first-child {
  border-top: 0;
}

.trial-link-row > div:first-child {
  display: grid;
  min-width: 0;
}

.trial-link-row > div:first-child span {
  overflow: hidden;
  color: var(--muted);
  font-size: 0.8rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.trial-link-meta {
  display: grid;
  min-width: 0;
}

.trial-link-meta time {
  color: var(--muted);
  font-size: 0.76rem;
}

.trial-register-note {
  margin: 0;
  color: var(--muted);
  font-size: 0.86rem;
}

.trial-register-note a {
  color: var(--accent-strong);
  font-weight: 700;
}

@media (max-width: 960px) {
  .trial-page {
    grid-template-columns: 1fr;
  }

  .trial-story {
    min-height: 24rem;
  }

  .trial-copy h1 {
    max-width: 15ch;
    font-size: clamp(2.4rem, 9vw, 4rem);
  }
}

@media (max-width: 640px) {
  .trial-topbar {
    align-items: flex-start;
  }

  .trial-session-actions {
    flex-direction: column;
    align-items: stretch;
  }

  .trial-login-link,
  .trial-register-link {
    text-align: center;
  }

  .trial-workspace {
    padding: 1rem;
  }

  .trial-heading,
  .trial-result {
    align-items: stretch;
    flex-direction: column;
  }

  .trial-form-row,
  .trial-link-row {
    grid-template-columns: 1fr;
  }

  .trial-link-row {
    gap: 0.45rem;
  }

  .trial-facts {
    display: none;
  }
}
</style>
