<script setup lang="ts">
import { computed, onMounted, reactive, shallowRef } from 'vue'

import { createAPIKey, listAPIKeys, revokeAPIKey } from '@/api'
import { ApiError } from '@/api/http'
import type { APIKeyRecord, APIKeyScope } from '@/api/types'
import { formatDateTime } from '@/utils/format'

const scopeOptions: Array<{
  value: APIKeyScope
  label: string
  description: string
}> = [
  {
    value: 'links:read',
    label: '读取短链',
    description: '查询链接列表、详情和二维码。',
  },
  {
    value: 'links:write',
    label: '管理短链',
    description: '创建、修改和删除短链。',
  },
  {
    value: 'analytics:read',
    label: '读取分析',
    description: '读取单链接和工作空间点击分析。',
  },
]

const form = reactive({
  name: '',
  scopes: scopeOptions.map((item) => item.value) as APIKeyScope[],
})
const keys = shallowRef<APIKeyRecord[]>([])
const loading = shallowRef(true)
const creating = shallowRef(false)
const revokingID = shallowRef('')
const errorMessage = shallowRef('')
const notice = shallowRef('')
const newSecret = shallowRef('')
const newPrefix = shallowRef('')

const activeCount = computed(
  () => keys.value.filter((key) => !key.revoked_at).length,
)
const apiOrigin = window.location.origin
const curlExample = computed(() => {
  const secret = newSecret.value || 'sl_live_your_api_key'
  return `curl "${apiOrigin}/api/v1/analytics/overview?days=7" \\
  -H "Authorization: Bearer ${secret}"`
})

onMounted(load)

// load 加载当前工作空间 API Key
async function load(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    keys.value = (await listAPIKeys()).items
  } catch (error) {
    errorMessage.value =
      error instanceof ApiError ? error.message : '加载 API Key 失败'
  } finally {
    loading.value = false
  }
}

// submit 创建 API Key，并显示唯一一次明文
async function submit(): Promise<void> {
  if (!form.name.trim() || form.scopes.length === 0 || creating.value) {
    return
  }
  creating.value = true
  errorMessage.value = ''
  notice.value = ''
  try {
    const result = await createAPIKey(form.name.trim(), form.scopes)
    keys.value = [result.key, ...keys.value]
    newSecret.value = result.secret
    newPrefix.value = result.key.prefix
    form.name = ''
    notice.value = 'API Key 已创建，请立即保存明文 Secret。'
  } catch (error) {
    errorMessage.value =
      error instanceof ApiError ? error.message : '创建 API Key 失败'
  } finally {
    creating.value = false
  }
}

// revoke 撤销 API Key
async function revoke(key: APIKeyRecord): Promise<void> {
  if (!window.confirm(`撤销 API Key「${key.name}」？撤销后无法恢复。`)) {
    return
  }
  revokingID.value = key.id
  errorMessage.value = ''
  try {
    await revokeAPIKey(key.id)
    const revokedAt = new Date().toISOString()
    keys.value = keys.value.map((item) =>
      item.id === key.id ? { ...item, revoked_at: revokedAt } : item,
    )
    if (newPrefix.value === key.prefix) {
      newSecret.value = ''
    }
    notice.value = `已撤销 ${key.name}`
  } catch (error) {
    errorMessage.value =
      error instanceof ApiError ? error.message : '撤销 API Key 失败'
  } finally {
    revokingID.value = ''
  }
}

// copySecret 复制完整 Secret 或 curl 示例
async function copyValue(value: string, successMessage: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(value)
    notice.value = successMessage
  } catch {
    notice.value = '浏览器未允许复制，请手动选择内容'
  }
}

// toggleScope 更新创建表单中的作用域选择
function toggleScope(scope: APIKeyScope): void {
  form.scopes = form.scopes.includes(scope)
    ? form.scopes.filter((item) => item !== scope)
    : [...form.scopes, scope]
}

// statusText 返回 API Key 当前状态
function statusText(key: APIKeyRecord): string {
  if (key.revoked_at) {
    return '已撤销'
  }
  return '有效'
}
</script>

<template>
  <div class="page-stack">
    <header class="page-header developer-header">
      <div>
        <p class="section-kicker">程序化接入</p>
        <h1>开发者</h1>
        <p>创建带作用域的 API Key，在脚本或其他服务中调用短链平台。</p>
      </div>
      <a class="secondary-button" href="/openapi.yaml" target="_blank" rel="noreferrer">
        查看 OpenAPI
      </a>
    </header>

    <p v-if="notice" class="inline-notice" role="status">{{ notice }}</p>
    <p v-if="errorMessage" class="form-error" role="alert">{{ errorMessage }}</p>

    <section v-if="newSecret" class="panel api-secret-panel" aria-labelledby="new-secret-title">
      <div>
        <p class="section-kicker">只显示一次</p>
        <h2 id="new-secret-title">保存这个 API Key</h2>
        <p>关闭提示后无法再次查看完整 Secret，只能重新创建或撤销。</p>
      </div>
      <code class="api-secret-value">{{ newSecret }}</code>
      <div class="api-secret-actions">
        <button
          class="primary-button"
          type="button"
          @click="copyValue(newSecret, 'API Key 已复制')"
        >
          复制 API Key
        </button>
        <button class="secondary-button" type="button" @click="newSecret = ''">
          我已保存
        </button>
      </div>
    </section>

    <div class="developer-grid">
      <section class="panel">
        <div class="section-heading-row">
          <div>
            <h2>创建 API Key</h2>
            <p>只授予实际需要的作用域。</p>
          </div>
        </div>

        <form class="api-key-form" @submit.prevent="submit">
          <label class="field">
            <span>名称</span>
            <input
              v-model="form.name"
              type="text"
              maxlength="64"
              placeholder="例如：部署脚本"
              required
            />
          </label>

          <fieldset class="scope-fieldset">
            <legend>作用域</legend>
            <label v-for="option in scopeOptions" :key="option.value" class="scope-option">
              <input
                type="checkbox"
                :checked="form.scopes.includes(option.value)"
                @change="toggleScope(option.value)"
              />
              <span>
                <strong>{{ option.label }}</strong>
                <small><code>{{ option.value }}</code> · {{ option.description }}</small>
              </span>
            </label>
          </fieldset>

          <button class="primary-button" type="submit" :disabled="creating">
            {{ creating ? '正在创建…' : '创建 API Key' }}
          </button>
        </form>
      </section>

      <section class="panel api-usage-panel">
        <p class="section-kicker">调用示例</p>
        <h2>Bearer 认证</h2>
        <p>API Key 可以放在 <code>Authorization: Bearer</code> 或 <code>X-API-Key</code>。</p>
        <pre><code>{{ curlExample }}</code></pre>
        <button
          class="secondary-button"
          type="button"
          @click="copyValue(curlExample, 'curl 示例已复制')"
        >
          复制 curl
        </button>
      </section>
    </div>

    <section class="panel">
      <div class="section-heading-row">
        <div>
          <h2>API Key 列表</h2>
          <p>{{ activeCount }} 个有效 Key。Secret 不会再次显示。</p>
        </div>
      </div>

      <p v-if="loading" class="quiet-state" role="status">正在加载 API Key…</p>
      <div v-else-if="keys.length === 0" class="empty-state">
        <h3>还没有 API Key</h3>
        <p>创建后可以在 curl、脚本或服务端程序中使用。</p>
      </div>
      <ul v-else class="api-key-list">
        <li v-for="key in keys" :key="key.id" class="api-key-row">
          <div class="api-key-main">
            <span class="status-pill" :class="{ 'is-disabled': key.revoked_at }">
              {{ statusText(key) }}
            </span>
            <div>
              <strong>{{ key.name }}</strong>
              <code>{{ key.prefix }}••••••</code>
            </div>
          </div>
          <div class="api-key-scopes">
            <code v-for="scope in key.scopes" :key="scope">{{ scope }}</code>
          </div>
          <div class="api-key-meta">
            <span>
              最近使用：{{ key.last_used_at ? formatDateTime(key.last_used_at) : '从未' }}
            </span>
            <span>创建：{{ formatDateTime(key.created_at) }}</span>
          </div>
          <button
            v-if="!key.revoked_at"
            class="small-button danger-button"
            type="button"
            :disabled="revokingID === key.id"
            @click="revoke(key)"
          >
            {{ revokingID === key.id ? '撤销中…' : '撤销' }}
          </button>
        </li>
      </ul>
    </section>
  </div>
</template>

<style scoped>
.developer-header {
  align-items: flex-end;
}

.developer-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.05fr) minmax(18rem, 0.95fr);
  gap: 1rem;
}

.api-key-form {
  display: grid;
  gap: 1rem;
  margin-top: 1rem;
}

.scope-fieldset {
  display: grid;
  gap: 0.65rem;
  padding: 0;
  border: 0;
}

.scope-fieldset legend {
  margin-bottom: 0.45rem;
  color: var(--muted-strong);
  font-size: 0.82rem;
  font-weight: 700;
}

.scope-option {
  display: flex;
  gap: 0.7rem;
  align-items: flex-start;
  padding: 0.75rem;
  border: 1px solid var(--line);
  border-radius: var(--radius-sm);
}

.scope-option input {
  margin-top: 0.2rem;
}

.scope-option span {
  display: grid;
  gap: 0.18rem;
}

.scope-option small {
  color: var(--muted);
}

.api-usage-panel {
  display: grid;
  align-content: start;
}

.api-usage-panel pre {
  overflow-x: auto;
  padding: 1rem;
  border-radius: var(--radius-sm);
  background: var(--rail-deep);
  color: #dff2ed;
  font: 0.8rem/1.65 var(--font-mono);
}

.api-secret-panel {
  display: grid;
  gap: 1rem;
  border-color: #9ecdc2;
  background: var(--accent-soft);
}

.api-secret-value {
  display: block;
  overflow-x: auto;
  padding: 0.9rem;
  border: 1px solid #9ecdc2;
  border-radius: var(--radius-sm);
  background: var(--surface-strong);
  font: 0.82rem/1.5 var(--font-mono);
  white-space: nowrap;
}

.api-secret-actions {
  display: flex;
  gap: 0.7rem;
}

.api-key-list {
  display: grid;
  padding: 0;
  margin: 1rem 0 0;
  list-style: none;
}

.api-key-row {
  display: grid;
  grid-template-columns: minmax(13rem, 1fr) minmax(10rem, 0.7fr) minmax(12rem, 0.8fr) auto;
  gap: 1rem;
  align-items: center;
  padding: 1rem 0;
  border-top: 1px solid var(--line);
}

.api-key-row:first-child {
  border-top: 0;
}

.api-key-main {
  display: flex;
  align-items: center;
  gap: 0.7rem;
}

.api-key-main div,
.api-key-meta {
  display: grid;
  gap: 0.15rem;
  min-width: 0;
}

.api-key-main code {
  color: var(--muted);
  font-size: 0.76rem;
}

.api-key-scopes {
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem;
}

.api-key-scopes code {
  padding: 0.18rem 0.4rem;
  border-radius: 0.3rem;
  background: var(--accent-soft);
  color: var(--accent-strong);
  font-size: 0.7rem;
}

.api-key-meta span {
  color: var(--muted);
  font-size: 0.76rem;
}

@media (max-width: 1000px) {
  .developer-grid,
  .api-key-row {
    grid-template-columns: 1fr;
  }
}
</style>
