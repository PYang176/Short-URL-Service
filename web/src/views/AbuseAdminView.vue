<script setup lang="ts">
import { onMounted, reactive, shallowRef } from 'vue'

import {
  addBlockedDomain,
  listAbuseReports,
  listBlockedDomains,
  removeBlockedDomain,
  resolveAbuseReport,
} from '@/api'
import { ApiError } from '@/api/http'
import type { AbuseReport, BlockedDomain } from '@/api/types'
import { formatDateTime } from '@/utils/format'

const reports = shallowRef<AbuseReport[]>([])
const blockedDomains = shallowRef<BlockedDomain[]>([])
const loading = shallowRef(true)
const resolvingID = shallowRef('')
const removingID = shallowRef('')
const addingDomain = shallowRef(false)
const errorMessage = shallowRef('')
const notice = shallowRef('')
const blockForm = reactive({
  hostname: '',
  reason: '',
})

onMounted(load)

// load 并行加载待处理举报和域名黑名单
async function load(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    const [reportResult, blockedResult] = await Promise.all([
      listAbuseReports(),
      listBlockedDomains(),
    ])
    reports.value = reportResult.items
    blockedDomains.value = blockedResult.items
  } catch (error) {
    errorMessage.value =
      error instanceof ApiError ? error.message : '加载治理数据失败'
  } finally {
    loading.value = false
  }
}

// resolve 处理举报并立即从待处理队列移除
async function resolve(
  report: AbuseReport,
  decision: 'takedown' | 'reject',
): Promise<void> {
  const actionText = decision === 'takedown' ? '下架短链' : '驳回举报'
  if (!window.confirm(`确认${actionText} /${report.link_slug}？`)) {
    return
  }
  resolvingID.value = report.id
  errorMessage.value = ''
  notice.value = ''
  try {
    const note =
      decision === 'takedown' ? '管理员审核确认违规' : '管理员审核后驳回'
    await resolveAbuseReport(report.id, decision, note)
    reports.value = reports.value.filter((item) => item.id !== report.id)
    notice.value = `${actionText}成功`
  } catch (error) {
    errorMessage.value =
      error instanceof ApiError ? error.message : `${actionText}失败`
  } finally {
    resolvingID.value = ''
  }
}

// addBlock 添加目标域名黑名单
async function addBlock(): Promise<void> {
  if (
    !blockForm.hostname.trim() ||
    !blockForm.reason.trim() ||
    addingDomain.value
  ) {
    return
  }
  addingDomain.value = true
  errorMessage.value = ''
  notice.value = ''
  try {
    const domain = await addBlockedDomain(
      blockForm.hostname.trim(),
      blockForm.reason.trim(),
    )
    blockedDomains.value = [
      domain,
      ...blockedDomains.value.filter((item) => item.id !== domain.id),
    ]
    blockForm.hostname = ''
    blockForm.reason = ''
    notice.value = '域名黑名单已添加'
  } catch (error) {
    errorMessage.value =
      error instanceof ApiError ? error.message : '添加域名黑名单失败'
  } finally {
    addingDomain.value = false
  }
}

// removeBlock 停用指定域名黑名单
async function removeBlock(domain: BlockedDomain): Promise<void> {
  if (!window.confirm(`解除 ${domain.hostname} 的目标阻断？`)) {
    return
  }
  removingID.value = domain.id
  errorMessage.value = ''
  try {
    await removeBlockedDomain(domain.id)
    blockedDomains.value = blockedDomains.value.filter(
      (item) => item.id !== domain.id,
    )
    notice.value = `已解除 ${domain.hostname}`
  } catch (error) {
    errorMessage.value =
      error instanceof ApiError ? error.message : '解除域名黑名单失败'
  } finally {
    removingID.value = ''
  }
}

// reasonText 返回举报原因的中文文案
function reasonText(reason: string): string {
  switch (reason) {
    case 'phishing':
      return '钓鱼欺诈'
    case 'malware':
      return '恶意软件'
    case 'spam':
      return '垃圾推广'
    case 'impersonation':
      return '仿冒身份'
    default:
      return '其他违规'
  }
}
</script>

<template>
  <div class="page-stack">
    <header class="page-header">
      <div>
        <p class="section-kicker">平台治理</p>
        <h1>滥用审核</h1>
        <p>处理访客举报，并阻断高风险目标域名。</p>
      </div>
      <button class="secondary-button" type="button" @click="load">刷新队列</button>
    </header>

    <p v-if="notice" class="inline-notice" role="status">{{ notice }}</p>
    <p v-if="errorMessage" class="form-error" role="alert">{{ errorMessage }}</p>

    <div class="governance-grid">
      <section class="panel report-panel" aria-labelledby="report-queue-title">
        <div class="section-heading-row">
          <div>
            <h2 id="report-queue-title">待处理举报</h2>
            <p>{{ reports.length }} 条记录。下架操作会同步失效 Redis 跳转缓存。</p>
          </div>
        </div>

        <p v-if="loading" class="quiet-state" role="status">正在加载举报…</p>
        <div v-else-if="reports.length === 0" class="empty-state">
          <h3>当前没有待处理举报</h3>
          <p>新的公开举报会出现在这里。</p>
        </div>
        <ul v-else class="report-list">
          <li v-for="report in reports" :key="report.id" class="report-row">
            <div class="report-main">
              <span class="status-pill is-warning">{{ reasonText(report.reason) }}</span>
              <div>
                <strong>{{ report.link_title || `/${report.link_slug}` }}</strong>
                <code>{{ report.domain_hostname }}/{{ report.link_slug }}</code>
              </div>
            </div>
            <p class="report-destination">{{ report.destination_url }}</p>
            <p v-if="report.details" class="report-details">{{ report.details }}</p>
            <div class="report-footer">
              <time>{{ formatDateTime(report.created_at) }}</time>
              <div class="report-actions">
                <button
                  class="small-button danger-button"
                  type="button"
                  :disabled="resolvingID === report.id"
                  @click="resolve(report, 'takedown')"
                >
                  下架短链
                </button>
                <button
                  class="small-button"
                  type="button"
                  :disabled="resolvingID === report.id"
                  @click="resolve(report, 'reject')"
                >
                  驳回
                </button>
              </div>
            </div>
          </li>
        </ul>
      </section>

      <section class="panel block-panel" aria-labelledby="blocklist-title">
        <div class="section-heading-row">
          <div>
            <h2 id="blocklist-title">目标域名黑名单</h2>
            <p>规则会自动匹配子域名，并阻止后续创建和修改。</p>
          </div>
        </div>

        <form class="block-form" @submit.prevent="addBlock">
          <label class="field">
            <span>域名或 IP</span>
            <input
              v-model="blockForm.hostname"
              type="text"
              maxlength="253"
              placeholder="例如：phishing.example"
              required
            />
          </label>
          <label class="field">
            <span>阻断原因</span>
            <input
              v-model="blockForm.reason"
              type="text"
              maxlength="500"
              placeholder="例如：钓鱼页面"
              required
            />
          </label>
          <button class="primary-button" type="submit" :disabled="addingDomain">
            {{ addingDomain ? '添加中…' : '添加阻断规则' }}
          </button>
        </form>

        <p v-if="loading" class="quiet-state" role="status">正在加载黑名单…</p>
        <p v-else-if="blockedDomains.length === 0" class="quiet-state">
          当前没有目标域名阻断规则。
        </p>
        <ul v-else class="block-list">
          <li v-for="domain in blockedDomains" :key="domain.id" class="block-row">
            <div>
              <strong>{{ domain.hostname }}</strong>
              <span>{{ domain.reason }}</span>
            </div>
            <button
              class="small-button danger-button"
              type="button"
              :disabled="removingID === domain.id"
              @click="removeBlock(domain)"
            >
              {{ removingID === domain.id ? '解除中…' : '解除' }}
            </button>
          </li>
        </ul>
      </section>
    </div>
  </div>
</template>

<style scoped>
.governance-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.25fr) minmax(20rem, 0.75fr);
  gap: 1rem;
  align-items: start;
}

.report-list,
.block-list {
  display: grid;
  padding: 0;
  margin: 1rem 0 0;
  list-style: none;
}

.report-row,
.block-row {
  padding: 1rem 0;
  border-top: 1px solid var(--line);
}

.report-row:first-child,
.block-row:first-child {
  border-top: 0;
}

.report-main {
  display: flex;
  align-items: flex-start;
  gap: 0.75rem;
}

.report-main > div {
  display: grid;
  min-width: 0;
  gap: 0.2rem;
}

.report-main code {
  overflow: hidden;
  color: var(--accent-strong);
  font-size: 0.76rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.status-pill.is-warning {
  border-color: #e0b7a9;
  background: var(--danger-soft);
  color: var(--danger);
}

.report-destination {
  overflow: hidden;
  margin: 0.55rem 0 0;
  color: var(--muted);
  font-size: 0.78rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.report-details {
  padding: 0.7rem;
  margin: 0.75rem 0 0;
  border-left: 3px solid var(--line-strong);
  background: var(--surface);
  font-size: 0.82rem;
}

.report-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  margin-top: 0.75rem;
}

.report-footer time {
  color: var(--muted);
  font-size: 0.76rem;
}

.report-actions {
  display: flex;
  gap: 0.45rem;
}

.block-form {
  display: grid;
  gap: 0.8rem;
  margin-top: 1rem;
}

.block-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
}

.block-row > div {
  display: grid;
  min-width: 0;
  gap: 0.15rem;
}

.block-row strong {
  overflow-wrap: anywhere;
  font-family: var(--font-mono);
  font-size: 0.82rem;
}

.block-row span {
  color: var(--muted);
  font-size: 0.76rem;
}

@media (max-width: 980px) {
  .governance-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 620px) {
  .report-footer {
    align-items: flex-start;
    flex-direction: column;
  }

  .report-actions {
    width: 100%;
  }

  .report-actions button {
    flex: 1;
  }
}
</style>
