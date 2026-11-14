<script setup lang="ts">
import { reactive, shallowRef } from 'vue'

import { exportAnalytics } from '@/api'
import { ApiError } from '@/api/http'
import type { AnalyticsExportFilter } from '@/api/types'

const { linkId = '' } = defineProps<{
  linkId?: string
}>()

const filter = reactive<AnalyticsExportFilter>({
  link_id: linkId || undefined,
  days: 7,
  visitor: 'all',
  device: '',
  country: '',
  browser: '',
  utm_source: '',
})
const exporting = shallowRef(false)
const errorMessage = shallowRef('')
const notice = shallowRef('')

// download 请求流式 CSV 并交给浏览器保存
async function download(): Promise<void> {
  if (exporting.value) {
    return
  }
  exporting.value = true
  errorMessage.value = ''
  notice.value = ''
  try {
    const blob = await exportAnalytics({
      ...filter,
      link_id: linkId || undefined,
    })
    const anchor = document.createElement('a')
    const url = URL.createObjectURL(blob)
    const scope = linkId ? 'link' : 'workspace'
    const date = new Date().toISOString().slice(0, 10)
    anchor.href = url
    anchor.download = `shorturl-${scope}-${date}.csv`
    anchor.click()
    URL.revokeObjectURL(url)
    notice.value = '点击明细已导出'
  } catch (error) {
    errorMessage.value =
      error instanceof ApiError ? error.message : '导出点击明细失败'
  } finally {
    exporting.value = false
  }
}
</script>

<template>
  <section class="panel analytics-export-panel" aria-labelledby="analytics-export-title">
    <div class="section-heading-row">
      <div>
        <h2 id="analytics-export-title">导出访问明细</h2>
        <p>按条件组合筛选后导出 CSV，不包含原始 IP 或访客哈希。</p>
      </div>
      <span class="export-scope">{{ linkId ? '当前短链' : '整个工作空间' }}</span>
    </div>

    <form class="export-form" @submit.prevent="download">
      <label class="field">
        <span>时间范围</span>
        <select v-model="filter.days">
          <option :value="1">最近 1 天</option>
          <option :value="7">最近 7 天</option>
          <option :value="30">最近 30 天</option>
          <option :value="90">最近 90 天</option>
        </select>
      </label>
      <label class="field">
        <span>访问类型</span>
        <select v-model="filter.visitor">
          <option value="all">全部访问</option>
          <option value="human">仅真人</option>
          <option value="bot">仅机器人</option>
        </select>
      </label>
      <label class="field">
        <span>设备</span>
        <select v-model="filter.device">
          <option value="">全部设备</option>
          <option value="desktop">桌面</option>
          <option value="mobile">手机</option>
          <option value="tablet">平板</option>
        </select>
      </label>
      <label class="field">
        <span>浏览器</span>
        <select v-model="filter.browser">
          <option value="">全部浏览器</option>
          <option value="Chrome">Chrome</option>
          <option value="Edge">Edge</option>
          <option value="Safari">Safari</option>
          <option value="Firefox">Firefox</option>
          <option value="Opera">Opera</option>
        </select>
      </label>
      <label class="field">
        <span>国家代码，可选</span>
        <input
          v-model="filter.country"
          type="text"
          maxlength="2"
          placeholder="CN"
        />
      </label>
      <label class="field">
        <span>UTM Source，可选</span>
        <input
          v-model="filter.utm_source"
          type="text"
          maxlength="255"
          placeholder="newsletter"
        />
      </label>
      <button class="primary-button export-submit" type="submit" :disabled="exporting">
        {{ exporting ? '正在生成 CSV…' : '导出 CSV' }}
      </button>
    </form>

    <p v-if="notice" class="inline-notice" role="status">{{ notice }}</p>
    <p v-if="errorMessage" class="form-error" role="alert">{{ errorMessage }}</p>
  </section>
</template>

<style scoped>
.analytics-export-panel {
  display: grid;
  gap: 1rem;
}

.export-scope {
  padding: 0.28rem 0.55rem;
  border: 1px solid var(--line);
  border-radius: 999px;
  color: var(--muted-strong);
  font-size: 0.76rem;
  white-space: nowrap;
}

.export-form {
  display: grid;
  grid-template-columns: repeat(6, minmax(0, 1fr)) auto;
  gap: 0.7rem;
  align-items: end;
}

.export-submit {
  white-space: nowrap;
}

@media (max-width: 1100px) {
  .export-form {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}

@media (max-width: 700px) {
  .export-form {
    grid-template-columns: 1fr 1fr;
  }
}

@media (max-width: 480px) {
  .export-form {
    grid-template-columns: 1fr;
  }
}
</style>
