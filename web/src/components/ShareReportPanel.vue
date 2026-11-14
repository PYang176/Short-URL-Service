<script setup lang="ts">
import { onMounted, shallowRef } from 'vue'

import {
  createAnalyticsShare,
  listAnalyticsShares,
  revokeAnalyticsShare,
} from '@/api'
import { ApiError } from '@/api/http'
import type { AnalyticsShare } from '@/api/types'
import { formatDateTime } from '@/utils/format'

const props = defineProps<{
  linkId: string
}>()

const shares = shallowRef<AnalyticsShare[]>([])
const loading = shallowRef(true)
const pending = shallowRef(false)
const errorMessage = shallowRef('')
const notice = shallowRef('')
const days = shallowRef<1 | 7 | 30>(7)
const createdURL = shallowRef('')

onMounted(loadShares)

// loadShares 加载当前短链有效的公开分享
async function loadShares(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    const result = await listAnalyticsShares(props.linkId)
    shares.value = result.items
  } catch (error) {
    errorMessage.value =
      error instanceof ApiError ? error.message : '加载公开分享失败'
  } finally {
    loading.value = false
  }
}

// createShare 创建固定分析范围的公开分享链接
async function createShare(): Promise<void> {
  if (pending.value) {
    return
  }
  pending.value = true
  errorMessage.value = ''
  notice.value = ''
  try {
    const result = await createAnalyticsShare(props.linkId, days.value)
    shares.value = [result.share, ...shares.value]
    createdURL.value = `${window.location.origin}/share/${result.token}`
    notice.value = '公开分享已创建，链接只显示一次。'
  } catch (error) {
    errorMessage.value =
      error instanceof ApiError ? error.message : '创建公开分享失败'
  } finally {
    pending.value = false
  }
}

// copyCreatedURL 复制刚创建的公开分享链接
async function copyCreatedURL(): Promise<void> {
  try {
    await navigator.clipboard.writeText(createdURL.value)
    notice.value = '公开分享链接已复制'
  } catch {
    notice.value = '浏览器未允许复制，请手动选择链接'
  }
}

// revokeShare 撤销公开分享并立即停止访问
async function revokeShare(item: AnalyticsShare): Promise<void> {
  if (!window.confirm('撤销后，该公开报告链接会立即失效。是否继续？')) {
    return
  }
  try {
    await revokeAnalyticsShare(props.linkId, item.id)
    shares.value = shares.value.filter((share) => share.id !== item.id)
    notice.value = '公开分享已撤销'
  } catch (error) {
    errorMessage.value =
      error instanceof ApiError ? error.message : '撤销公开分享失败'
  }
}
</script>

<template>
  <section class="panel share-report-panel" aria-labelledby="share-report-title">
    <div class="section-heading-row">
      <div>
        <h2 id="share-report-title">公开访问报告</h2>
        <p>创建只读链接分享访问数据，不包含访客身份和内部工作空间信息。</p>
      </div>
      <label class="compact-field">
        <span>分析范围</span>
        <select v-model="days">
          <option :value="1">最近 1 天</option>
          <option :value="7">最近 7 天</option>
          <option :value="30">最近 30 天</option>
        </select>
      </label>
    </div>

    <div class="share-create-row">
      <p>分享有效期与分析范围一致，过期或撤销后立即停止访问。</p>
      <button class="primary-button" type="button" :disabled="pending" @click="createShare">
        {{ pending ? '正在创建…' : '创建公开分享' }}
      </button>
    </div>

    <p v-if="notice" class="inline-notice" role="status">{{ notice }}</p>
    <p v-if="errorMessage" class="form-error" role="alert">{{ errorMessage }}</p>

    <div v-if="createdURL" class="created-share">
      <span>新分享链接</span>
      <input :value="createdURL" readonly aria-label="新公开分享链接" />
      <button class="secondary-button" type="button" @click="copyCreatedURL">复制</button>
    </div>

    <p v-if="loading" class="quiet-state" role="status">正在加载公开分享…</p>
    <p v-else-if="shares.length === 0" class="quiet-state">
      当前没有有效的公开分享。
    </p>
    <ul v-else class="share-list">
      <li v-for="item in shares" :key="item.id" class="share-row">
        <div>
          <strong>{{ item.days }} 天访问报告</strong>
          <span>有效期至 {{ formatDateTime(item.expires_at) }}</span>
        </div>
        <button class="small-button danger-button" type="button" @click="revokeShare(item)">
          撤销
        </button>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.share-report-panel {
  display: grid;
  gap: 1rem;
}

.share-report-panel p {
  margin: 0;
  color: var(--muted);
  font-size: 0.78rem;
}

.share-create-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
}

.created-share {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  gap: 0.65rem;
  align-items: center;
  padding: 0.75rem;
  border: 1px solid #a8d7cc;
  border-radius: var(--radius-sm);
  background: var(--accent-soft);
}

.created-share span {
  color: var(--accent-strong);
  font-size: 0.72rem;
  font-weight: 750;
}

.created-share input {
  min-width: 0;
  padding: 0.55rem 0.65rem;
  border: 1px solid var(--line-strong);
  border-radius: var(--radius-sm);
  background: #ffffff;
  color: var(--accent-strong);
  font-family: var(--font-mono);
  font-size: 0.76rem;
}

.share-list {
  display: grid;
  padding: 0;
  margin: 0;
  list-style: none;
}

.share-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  padding: 0.75rem 0;
  border-top: 1px solid var(--line);
}

.share-row:first-child {
  border-top: 0;
}

.share-row > div {
  display: grid;
}

.share-row span {
  color: var(--muted);
  font-size: 0.72rem;
}

@media (max-width: 620px) {
  .share-report-panel .section-heading-row,
  .share-create-row,
  .created-share {
    align-items: stretch;
    grid-template-columns: 1fr;
    flex-direction: column;
  }
}
</style>
