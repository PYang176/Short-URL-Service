<script setup lang="ts">
import { computed, onMounted, shallowRef } from 'vue'

import { addDomain, deleteDomain, listDomains, verifyDomain } from '@/api'
import { ApiError } from '@/api/http'
import type { Domain } from '@/api/types'
import { formatDateTime } from '@/utils/format'

const domains = shallowRef<Domain[]>([])
const hostname = shallowRef('')
const loading = shallowRef(true)
const adding = shallowRef(false)
const verifyingID = shallowRef('')
const errorMessage = shallowRef('')
const notice = shallowRef('')

const pendingCount = computed(
  () => domains.value.filter((domain) => !domain.verified).length,
)

onMounted(load)

// load 加载工作空间域名
async function load(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    domains.value = (await listDomains()).items
  } catch (error) {
    errorMessage.value = error instanceof ApiError ? error.message : '加载域名失败'
  } finally {
    loading.value = false
  }
}

// submit 添加待验证域名
async function submit(): Promise<void> {
  if (!hostname.value.trim() || adding.value) {
    return
  }
  adding.value = true
  errorMessage.value = ''
  try {
    const created = await addDomain(hostname.value.trim())
    domains.value = [created, ...domains.value]
    hostname.value = ''
    notice.value = '域名已添加，请配置 TXT 和 CNAME 记录'
  } catch (error) {
    errorMessage.value = error instanceof ApiError ? error.message : '添加域名失败'
  } finally {
    adding.value = false
  }
}

// verify 验证域名配置
async function verify(domain: Domain): Promise<void> {
  verifyingID.value = domain.id
  errorMessage.value = ''
  try {
    const updated = await verifyDomain(domain.id)
    replaceDomain(updated)
    notice.value = updated.verified ? '域名验证成功' : '域名尚未通过验证'
  } catch (error) {
    errorMessage.value =
      error instanceof ApiError ? error.message : '域名验证失败'
  } finally {
    verifyingID.value = ''
  }
}

// remove 删除未被链接使用的域名
async function remove(domain: Domain): Promise<void> {
  if (!window.confirm(`删除域名 ${domain.hostname}？`)) {
    return
  }
  try {
    await deleteDomain(domain.id)
    domains.value = domains.value.filter((item) => item.id !== domain.id)
    notice.value = `已删除 ${domain.hostname}`
  } catch (error) {
    errorMessage.value = error instanceof ApiError ? error.message : '删除域名失败'
  }
}

// replaceDomain 更新列表中的单个域名
function replaceDomain(updated: Domain): void {
  domains.value = domains.value.map((domain) =>
    domain.id === updated.id ? updated : domain,
  )
}

// statusText 返回域名状态文案
function statusText(domain: Domain): string {
  if (domain.verified) {
    return '已验证'
  }
  if (domain.status === 4) {
    return '验证失败'
  }
  return '待验证'
}
</script>

<template>
  <div class="page-stack">
    <header class="page-header">
      <div>
        <p class="section-kicker">品牌入口</p>
        <h1>自定义域名</h1>
        <p>验证域名所有权后，可以使用自己的品牌域名创建短链。</p>
      </div>
    </header>

    <section class="panel domain-add-panel">
      <div>
        <h2>添加域名</h2>
        <p>输入完整域名，不要包含协议和路径。</p>
      </div>
      <form class="domain-add-form" @submit.prevent="submit">
        <label class="field">
          <span class="visually-hidden">域名</span>
          <input
            v-model="hostname"
            type="text"
            inputmode="url"
            placeholder="go.example.com"
            autocomplete="off"
            required
          />
        </label>
        <button class="primary-button" type="submit" :disabled="adding">
          {{ adding ? '添加中…' : '添加域名' }}
        </button>
      </form>
      <p class="domain-mode-note">
        当前本地开发模式可直接点击“验证”；切换到 DNS 模式后会检查 TXT 与 CNAME。
      </p>
    </section>

    <p v-if="notice" class="inline-notice" role="status">{{ notice }}</p>
    <p v-if="errorMessage" class="form-error" role="alert">{{ errorMessage }}</p>

    <section class="panel">
      <div class="section-heading-row">
        <div>
          <h2>域名列表</h2>
          <p>{{ pendingCount }} 个域名尚未完成验证。</p>
        </div>
        <span class="toolbar-count">{{ domains.length }} 个域名</span>
      </div>

      <div v-if="loading" class="skeleton-stack" role="status" aria-label="正在加载域名">
        <span v-for="index in 3" :key="index" class="skeleton-row" />
      </div>
      <div v-else-if="domains.length === 0" class="empty-state">
        <h3>还没有自定义域名</h3>
        <p>添加域名后，可以把它作为短链接的品牌入口。</p>
      </div>
      <div v-else class="domain-list">
        <article v-for="domain in domains" :key="domain.id" class="domain-card">
          <div class="domain-summary">
            <span
              class="status-pill"
              :class="{ 'is-disabled': !domain.verified }"
            >
              {{ statusText(domain) }}
            </span>
            <div class="domain-identity">
              <h3>{{ domain.hostname }}</h3>
              <p>
                {{
                  domain.last_verified_at
                    ? `验证于 ${formatDateTime(domain.last_verified_at)}`
                    : '等待 DNS 配置'
                }}
              </p>
            </div>
            <div class="domain-actions">
              <button
                v-if="!domain.verified"
                class="primary-button"
                type="button"
                :disabled="verifyingID === domain.id"
                @click="verify(domain)"
              >
                {{ verifyingID === domain.id ? '验证中…' : '验证域名' }}
              </button>
              <button class="small-button danger-button" type="button" @click="remove(domain)">
                删除
              </button>
            </div>
          </div>

          <details v-if="!domain.verified" class="dns-instructions">
            <summary>查看 DNS 配置记录</summary>
            <dl>
              <div>
                <dt>TXT 名称</dt>
                <dd><code>{{ domain.txt_name }}</code></dd>
              </div>
              <div>
                <dt>TXT 值</dt>
                <dd><code>{{ domain.txt_value }}</code></dd>
              </div>
              <div>
                <dt>CNAME 目标</dt>
                <dd><code>{{ domain.cname }}</code></dd>
              </div>
            </dl>
          </details>
        </article>
      </div>
    </section>
  </div>
</template>
