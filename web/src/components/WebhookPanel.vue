<script setup lang="ts">
import { computed, onMounted, reactive, shallowRef } from 'vue'

import {
  createWebhook,
  disableWebhook,
  listWebhookDeliveries,
  listWebhooks,
} from '@/api'
import { ApiError } from '@/api/http'
import type { WebhookDelivery, WebhookEndpoint } from '@/api/types'
import { formatDateTime } from '@/utils/format'

const form = reactive({
  name: '',
  url: '',
})
const endpoints = shallowRef<WebhookEndpoint[]>([])
const deliveries = shallowRef<WebhookDelivery[]>([])
const selectedEndpointID = shallowRef('')
const loading = shallowRef(true)
const creating = shallowRef(false)
const deliveriesLoading = shallowRef(false)
const disablingID = shallowRef('')
const errorMessage = shallowRef('')
const notice = shallowRef('')
const newSecret = shallowRef('')

const activeCount = computed(
  () => endpoints.value.filter((endpoint) => endpoint.status === 1).length,
)
const signatureExample = `import hashlib
import hmac

body = request.get_data()
timestamp = request.headers["X-ShortURL-Timestamp"]
message = timestamp.encode() + b"." + body
expected = "sha256=" + hmac.new(
    secret.encode(), message, hashlib.sha256
).hexdigest()
hmac.compare_digest(
    expected, request.headers["X-ShortURL-Signature"]
)`

onMounted(load)

// load 加载当前工作空间 Webhook 端点
async function load(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    endpoints.value = (await listWebhooks()).items
  } catch (error) {
    errorMessage.value =
      error instanceof ApiError ? error.message : '加载 Webhook 失败'
  } finally {
    loading.value = false
  }
}

// submit 创建 Webhook 并展示唯一一次 Secret
async function submit(): Promise<void> {
  if (!form.name.trim() || !form.url.trim() || creating.value) {
    return
  }
  creating.value = true
  errorMessage.value = ''
  notice.value = ''
  try {
    const result = await createWebhook(form.name.trim(), form.url.trim())
    endpoints.value = [result.endpoint, ...endpoints.value]
    newSecret.value = result.secret
    form.name = ''
    form.url = ''
    notice.value = 'Webhook 已创建，请立即保存签名 Secret。'
  } catch (error) {
    errorMessage.value =
      error instanceof ApiError ? error.message : '创建 Webhook 失败'
  } finally {
    creating.value = false
  }
}

// disable 停用端点并同步界面状态
async function disable(endpoint: WebhookEndpoint): Promise<void> {
  if (!window.confirm(`停用 Webhook「${endpoint.name}」？待投递任务也会终止。`)) {
    return
  }
  disablingID.value = endpoint.id
  errorMessage.value = ''
  try {
    await disableWebhook(endpoint.id)
    const disabledAt = new Date().toISOString()
    endpoints.value = endpoints.value.map((item) =>
      item.id === endpoint.id
        ? { ...item, status: 2, disabled_at: disabledAt }
        : item,
    )
    if (selectedEndpointID.value === endpoint.id) {
      deliveries.value = []
      selectedEndpointID.value = ''
    }
    notice.value = `已停用 ${endpoint.name}`
  } catch (error) {
    errorMessage.value =
      error instanceof ApiError ? error.message : '停用 Webhook 失败'
  } finally {
    disablingID.value = ''
  }
}

// toggleDeliveries 展开或收起端点近期投递记录
async function toggleDeliveries(endpoint: WebhookEndpoint): Promise<void> {
  if (selectedEndpointID.value === endpoint.id) {
    selectedEndpointID.value = ''
    deliveries.value = []
    return
  }
  selectedEndpointID.value = endpoint.id
  deliveries.value = []
  deliveriesLoading.value = true
  errorMessage.value = ''
  try {
    deliveries.value = (await listWebhookDeliveries(endpoint.id)).items
  } catch (error) {
    errorMessage.value =
      error instanceof ApiError ? error.message : '加载投递记录失败'
  } finally {
    deliveriesLoading.value = false
  }
}

// copySecret 复制签名 Secret
async function copySecret(): Promise<void> {
  try {
    await navigator.clipboard.writeText(newSecret.value)
    notice.value = 'Webhook Secret 已复制'
  } catch {
    notice.value = '浏览器未允许复制，请手动选择内容'
  }
}

// endpointStatusText 返回端点状态文案
function endpointStatusText(endpoint: WebhookEndpoint): string {
  return endpoint.status === 2 ? '已停用' : '接收中'
}

// deliveryStatusText 返回投递状态文案
function deliveryStatusText(status: number): string {
  switch (status) {
    case 1:
      return '等待投递'
    case 2:
      return '投递中'
    case 3:
      return '已送达'
    case 4:
      return '已终止'
    default:
      return '未知状态'
  }
}

// deliveryStatusClass 返回投递状态所需的样式类
function deliveryStatusClass(status: number): string {
  if (status === 3) {
    return 'is-success'
  }
  if (status === 4) {
    return 'is-dead'
  }
  return 'is-pending'
}
</script>

<template>
  <section class="webhook-feature" aria-labelledby="webhook-title">
    <div class="webhook-heading">
      <div>
        <p class="section-kicker">事件投递</p>
        <h2 id="webhook-title">Webhook</h2>
        <p>
          短链创建、修改或删除后，通过签名回调通知其他服务。每个事件使用独立
          Event ID，失败会按指数退避重试。
        </p>
      </div>
      <span class="webhook-count">{{ activeCount }} 个接收中</span>
    </div>

    <p v-if="notice" class="inline-notice" role="status">{{ notice }}</p>
    <p v-if="errorMessage" class="form-error" role="alert">{{ errorMessage }}</p>

    <section v-if="newSecret" class="panel webhook-secret-panel" aria-labelledby="webhook-secret-title">
      <div>
        <p class="section-kicker">只显示一次</p>
        <h2 id="webhook-secret-title">保存签名 Secret</h2>
        <p>接收端用它校验 HMAC-SHA256。关闭后无法再次查看完整 Secret。</p>
      </div>
      <code class="webhook-secret-value">{{ newSecret }}</code>
      <div class="webhook-secret-actions">
        <button class="primary-button" type="button" @click="copySecret">
          复制 Secret
        </button>
        <button class="secondary-button" type="button" @click="newSecret = ''">
          我已保存
        </button>
      </div>
    </section>

    <div class="webhook-grid">
      <section class="panel">
        <div class="section-heading-row">
          <div>
            <h2>创建端点</h2>
            <p>本地演示可先接收请求，再观察签名和投递状态。</p>
          </div>
        </div>

        <form class="webhook-form" @submit.prevent="submit">
          <label class="field">
            <span>名称</span>
            <input
              v-model="form.name"
              type="text"
              maxlength="64"
              placeholder="例如：订单服务"
              required
            />
          </label>
          <label class="field">
            <span>回调地址</span>
            <input
              v-model="form.url"
              type="url"
              maxlength="2048"
              placeholder="http://host.docker.internal:9010/hook"
              required
            />
          </label>
          <button class="primary-button" type="submit" :disabled="creating">
            {{ creating ? '正在创建…' : '创建 Webhook' }}
          </button>
        </form>
      </section>

      <aside class="panel webhook-protocol-panel" aria-labelledby="webhook-protocol-title">
        <p class="section-kicker">验证方式</p>
        <h2 id="webhook-protocol-title">HMAC 校验</h2>
        <p>签名原文是 <code>timestamp + "." + body</code>，请求头以 <code>sha256=</code> 开头。</p>
        <pre><code>{{ signatureExample }}</code></pre>
      </aside>
    </div>

    <section class="panel webhook-list-panel" aria-labelledby="webhook-list-title">
      <div class="section-heading-row">
        <div>
          <h2 id="webhook-list-title">端点与投递记录</h2>
          <p>最近记录保存在 MySQL Outbox 中，页面只展示状态，不暴露 Secret。</p>
        </div>
      </div>

      <p v-if="loading" class="quiet-state" role="status">正在加载 Webhook…</p>
      <div v-else-if="endpoints.length === 0" class="empty-state">
        <h3>还没有 Webhook</h3>
        <p>创建端点后，新建一条短链即可触发 link.created 事件。</p>
      </div>
      <ul v-else class="webhook-list">
        <li v-for="endpoint in endpoints" :key="endpoint.id" class="webhook-row">
          <div class="webhook-row-main">
            <span
              class="status-pill"
              :class="{ 'is-disabled': endpoint.status === 2 }"
            >
              {{ endpointStatusText(endpoint) }}
            </span>
            <div class="webhook-identity">
              <strong>{{ endpoint.name }}</strong>
              <code>{{ endpoint.url }}</code>
              <small>
                创建：{{ formatDateTime(endpoint.created_at) }}
                <template v-if="endpoint.failure_count > 0">
                  · 连续失败 {{ endpoint.failure_count }} 次
                </template>
              </small>
            </div>
          </div>
          <div class="webhook-actions">
            <button
              class="small-button"
              type="button"
              :aria-expanded="selectedEndpointID === endpoint.id"
              @click="toggleDeliveries(endpoint)"
            >
              {{ selectedEndpointID === endpoint.id ? '收起记录' : '查看投递' }}
            </button>
            <button
              v-if="endpoint.status === 1"
              class="small-button danger-button"
              type="button"
              :disabled="disablingID === endpoint.id"
              @click="disable(endpoint)"
            >
              {{ disablingID === endpoint.id ? '停用中…' : '停用' }}
            </button>
          </div>

          <div
            v-if="selectedEndpointID === endpoint.id"
            class="delivery-panel"
            aria-live="polite"
          >
            <p v-if="deliveriesLoading" class="quiet-state" role="status">正在加载投递记录…</p>
            <div v-else-if="deliveries.length === 0" class="delivery-empty">
              暂无投递记录。创建或修改一条短链后会出现在这里。
            </div>
            <ol v-else class="delivery-list">
              <li v-for="delivery in deliveries" :key="delivery.id" class="delivery-row">
                <div>
                  <code>{{ delivery.event_type }}</code>
                  <small>{{ delivery.event_id }}</small>
                </div>
                <span
                  class="delivery-status"
                  :class="deliveryStatusClass(delivery.status)"
                >
                  {{ deliveryStatusText(delivery.status) }}
                </span>
                <div class="delivery-meta">
                  <span>尝试 {{ delivery.attempt_count }} 次</span>
                  <span v-if="delivery.response_status">
                    HTTP {{ delivery.response_status }}
                  </span>
                  <span v-if="delivery.delivered_at">
                    送达 {{ formatDateTime(delivery.delivered_at) }}
                  </span>
                  <span v-else>
                    下次 {{ formatDateTime(delivery.next_attempt_at) }}
                  </span>
                </div>
                <p v-if="delivery.last_error" class="delivery-error">
                  {{ delivery.last_error }}
                </p>
              </li>
            </ol>
          </div>
        </li>
      </ul>
    </section>
  </section>
</template>

<style scoped>
.webhook-feature {
  display: grid;
  gap: 1rem;
  padding-top: 0.6rem;
  border-top: 1px solid var(--line);
}

.webhook-heading {
  display: flex;
  align-items: end;
  justify-content: space-between;
  gap: 1rem;
}

.webhook-heading h2 {
  margin-bottom: 0.25rem;
}

.webhook-heading p:last-child {
  max-width: 68ch;
  margin-bottom: 0;
  color: var(--muted);
}

.webhook-count {
  flex: 0 0 auto;
  padding: 0.35rem 0.6rem;
  border: 1px solid var(--line);
  border-radius: 999px;
  background: var(--surface);
  color: var(--muted-strong);
  font-size: 0.76rem;
  font-weight: 700;
}

.webhook-secret-panel {
  display: grid;
  gap: 1rem;
  border-color: #9ecdc2;
  background: var(--accent-soft);
}

.webhook-secret-value {
  display: block;
  overflow-x: auto;
  padding: 0.9rem;
  border: 1px solid #9ecdc2;
  border-radius: var(--radius-sm);
  background: var(--surface-strong);
  font: 0.82rem/1.5 var(--font-mono);
  white-space: nowrap;
}

.webhook-secret-actions {
  display: flex;
  gap: 0.7rem;
}

.webhook-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.05fr) minmax(18rem, 0.95fr);
  gap: 1rem;
}

.webhook-form {
  display: grid;
  gap: 1rem;
  margin-top: 1rem;
}

.webhook-protocol-panel {
  display: grid;
  align-content: start;
}

.webhook-protocol-panel pre {
  overflow-x: auto;
  padding: 1rem;
  margin-bottom: 0;
  border-radius: var(--radius-sm);
  background: var(--rail-deep);
  color: #dff2ed;
  font: 0.76rem/1.65 var(--font-mono);
}

.webhook-list {
  display: grid;
  padding: 0;
  margin: 1rem 0 0;
  list-style: none;
}

.webhook-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 1rem;
  padding: 1rem 0;
  border-top: 1px solid var(--line);
}

.webhook-row:first-child {
  border-top: 0;
}

.webhook-row-main {
  display: flex;
  align-items: flex-start;
  gap: 0.75rem;
  min-width: 0;
}

.webhook-identity {
  display: grid;
  min-width: 0;
  gap: 0.18rem;
}

.webhook-identity code {
  overflow: hidden;
  color: var(--accent-strong);
  font-size: 0.78rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.webhook-identity small,
.delivery-row small {
  color: var(--muted);
  font-size: 0.74rem;
}

.webhook-actions {
  display: flex;
  flex-wrap: wrap;
  align-content: start;
  justify-content: flex-end;
  gap: 0.45rem;
}

.delivery-panel {
  grid-column: 1 / -1;
  padding: 0.85rem;
  border: 1px solid var(--line);
  border-radius: var(--radius-sm);
  background: var(--surface);
}

.delivery-list {
  display: grid;
  gap: 0.7rem;
  padding: 0;
  margin: 0;
  list-style: none;
}

.delivery-row {
  display: grid;
  grid-template-columns: minmax(12rem, 1fr) auto minmax(13rem, 0.8fr);
  gap: 0.8rem;
  align-items: center;
  padding: 0.7rem 0;
  border-top: 1px solid var(--line);
}

.delivery-row:first-child {
  border-top: 0;
}

.delivery-row > div:first-child {
  display: grid;
  min-width: 0;
  gap: 0.14rem;
}

.delivery-row code,
.delivery-row small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.delivery-status {
  padding: 0.24rem 0.5rem;
  border-radius: 999px;
  font-size: 0.72rem;
  font-weight: 700;
}

.delivery-status.is-success {
  background: var(--accent-soft);
  color: var(--accent-strong);
}

.delivery-status.is-pending {
  background: #f6eedf;
  color: #855d19;
}

.delivery-status.is-dead {
  background: var(--danger-soft);
  color: var(--danger);
}

.delivery-meta {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 0.3rem 0.7rem;
  color: var(--muted);
  font-size: 0.74rem;
}

.delivery-error {
  grid-column: 1 / -1;
  margin: 0;
  color: var(--danger);
  font-size: 0.76rem;
}

.delivery-empty {
  color: var(--muted);
  font-size: 0.82rem;
  text-align: center;
}

@media (max-width: 1000px) {
  .webhook-grid,
  .delivery-row {
    grid-template-columns: 1fr;
  }

  .delivery-meta {
    justify-content: flex-start;
  }
}

@media (max-width: 620px) {
  .webhook-heading,
  .webhook-row {
    align-items: flex-start;
    grid-template-columns: 1fr;
  }

  .webhook-heading {
    flex-direction: column;
  }

  .webhook-actions {
    justify-content: flex-start;
  }

  .webhook-secret-actions {
    flex-direction: column;
  }
}
</style>
