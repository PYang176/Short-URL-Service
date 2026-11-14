<script setup lang="ts">
import { shallowRef } from 'vue'

import { bulkCreateLinks } from '@/api'
import { ApiError } from '@/api/http'
import type { BulkCreateResponse } from '@/api/types'

const emit = defineEmits<{
  imported: [count: number]
}>()

const file = shallowRef<File | null>(null)
const importing = shallowRef(false)
const errorMessage = shallowRef('')
const result = shallowRef<BulkCreateResponse | null>(null)
const idempotencyKey = shallowRef('')

const templateHeaders = 'destination_url,alias,title,notes,redirect_code,starts_at,expires_at'
const maxFileBytes = 900 * 1024

// handleFile 选择新文件后重置本次批次和幂等键
function handleFile(event: Event): void {
  const input = event.target as HTMLInputElement
  file.value = input.files?.[0] ?? null
  idempotencyKey.value = crypto.randomUUID()
  errorMessage.value = ''
  result.value = null
}

// submit 上传 CSV 并展示逐行结果
async function submit(): Promise<void> {
  if (!file.value || importing.value) {
    return
  }
  if (file.value.size > maxFileBytes) {
    errorMessage.value = 'CSV 文件不能超过 900 KiB'
    return
  }
  importing.value = true
  errorMessage.value = ''
  try {
    result.value = await bulkCreateLinks(file.value, idempotencyKey.value)
    if (result.value.succeeded > 0) {
      emit('imported', result.value.succeeded)
    }
  } catch (error) {
    errorMessage.value =
      error instanceof ApiError ? error.message : '批量导入失败'
  } finally {
    importing.value = false
  }
}

// downloadTemplate 下载带示例行的 UTF-8 CSV 模板
function downloadTemplate(): void {
  const body = `${templateHeaders}
https://example.com/landing,bulklink,批量导入示例,,302,,
`
  const blob = new Blob([`\uFEFF${body}`], { type: 'text/csv;charset=utf-8' })
  const anchor = document.createElement('a')
  const url = URL.createObjectURL(blob)
  anchor.href = url
  anchor.download = 'shorturl-bulk-template.csv'
  anchor.click()
  URL.revokeObjectURL(url)
}
</script>

<template>
  <section class="panel bulk-import-panel" aria-labelledby="bulk-import-title">
    <div class="section-heading-row">
      <div>
        <h2 id="bulk-import-title">CSV 批量创建</h2>
        <p>最多 500 行；合法行会成功创建，失败行会单独返回原因。</p>
      </div>
      <button class="secondary-button" type="button" @click="downloadTemplate">
        下载 CSV 模板
      </button>
    </div>

    <form class="bulk-import-form" @submit.prevent="submit">
      <label class="field bulk-file-field">
        <span>CSV 文件</span>
        <input
          type="file"
          accept=".csv,text/csv"
          required
          @change="handleFile"
        />
      </label>
      <button class="primary-button" type="submit" :disabled="!file || importing">
        {{ importing ? '正在导入…' : '开始导入' }}
      </button>
    </form>

    <p class="bulk-columns">
      表头：<code>{{ templateHeaders }}</code>。时间使用 RFC3339，例如
      <code>2026-11-20T10:00:00+08:00</code>。
    </p>
    <p v-if="errorMessage" class="form-error" role="alert">{{ errorMessage }}</p>

    <section v-if="result" class="bulk-result" aria-labelledby="bulk-result-title">
      <div class="bulk-result-heading">
        <div>
          <h3 id="bulk-result-title">批次结果</h3>
          <p>
            批次 <code>{{ result.batch_id }}</code>，共 {{ result.total }} 行。
          </p>
        </div>
        <div class="bulk-counts">
          <span class="is-success">成功 {{ result.succeeded }}</span>
          <span class="is-failed">失败 {{ result.failed }}</span>
        </div>
      </div>

      <ol class="bulk-result-list" aria-live="polite">
        <li v-for="item in result.items" :key="item.row" class="bulk-result-row">
          <span class="bulk-row-number">第 {{ item.row }} 行</span>
          <span class="status-pill" :class="{ 'is-disabled': item.status === 'failed' }">
            {{ item.status === 'created' ? '成功' : '失败' }}
          </span>
          <div>
            <template v-if="item.link">
              <strong>{{ item.link.title || `/${item.link.slug}` }}</strong>
              <a :href="item.link.short_url" target="_blank" rel="noreferrer">
                {{ item.link.short_url }}
              </a>
            </template>
            <template v-else>
              <strong>{{ item.error_code }}</strong>
              <span>{{ item.error_message }}</span>
            </template>
          </div>
        </li>
      </ol>
    </section>
  </section>
</template>

<style scoped>
.bulk-import-panel {
  display: grid;
  gap: 1rem;
}

.bulk-import-form {
  display: flex;
  align-items: end;
  gap: 0.8rem;
}

.bulk-file-field {
  flex: 1;
}

.bulk-columns {
  margin: 0;
  color: var(--muted);
  font-size: 0.78rem;
  overflow-wrap: anywhere;
}

.bulk-columns code {
  color: var(--accent-strong);
}

.bulk-result {
  display: grid;
  gap: 0.8rem;
  padding-top: 1rem;
  border-top: 1px solid var(--line);
}

.bulk-result-heading,
.bulk-counts,
.bulk-result-row {
  display: flex;
  align-items: center;
  gap: 0.7rem;
}

.bulk-result-heading {
  justify-content: space-between;
}

.bulk-result-heading p {
  margin: 0.2rem 0 0;
  color: var(--muted);
  font-size: 0.78rem;
}

.bulk-counts span {
  padding: 0.25rem 0.5rem;
  border-radius: 999px;
  font-size: 0.74rem;
  font-weight: 700;
}

.bulk-counts .is-success {
  background: var(--accent-soft);
  color: var(--accent-strong);
}

.bulk-counts .is-failed {
  background: var(--danger-soft);
  color: var(--danger);
}

.bulk-result-list {
  display: grid;
  padding: 0;
  margin: 0;
  list-style: none;
}

.bulk-result-row {
  padding: 0.65rem 0;
  border-top: 1px solid var(--line);
}

.bulk-result-row:first-child {
  border-top: 0;
}

.bulk-row-number {
  min-width: 4.5rem;
  color: var(--muted);
  font-size: 0.76rem;
}

.bulk-result-row > div {
  display: grid;
  min-width: 0;
  gap: 0.15rem;
}

.bulk-result-row a,
.bulk-result-row span:last-child {
  overflow: hidden;
  color: var(--muted);
  font-size: 0.78rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 700px) {
  .bulk-import-form,
  .bulk-result-heading,
  .bulk-result-row {
    align-items: stretch;
    flex-direction: column;
  }
}
</style>
