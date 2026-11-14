<script setup lang="ts">
import { computed, onMounted, shallowRef } from 'vue'
import { RouterLink } from 'vue-router'

import { deleteLink } from '@/api'
import { ApiError } from '@/api/http'
import type { Link } from '@/api/types'
import BulkImportPanel from '@/components/BulkImportPanel.vue'
import LinkFormDialog from '@/components/LinkFormDialog.vue'
import { useLinks } from '@/composables/useLinks'
import {
  formatDateTime,
  shortDestination,
  statusLabel,
} from '@/utils/format'

const {
  links,
  loading,
  loadingMore,
  errorMessage,
  hasMore,
  load,
  loadMore,
  replaceLink,
  removeLink,
} = useLinks()

const query = shallowRef('')
const statusFilter = shallowRef('all')
const createOpen = shallowRef(false)
const bulkOpen = shallowRef(false)
const editingLink = shallowRef<Link | null>(null)
const notice = shallowRef('')

const filteredLinks = computed(() => {
  const keyword = query.value.trim().toLowerCase()
  return links.value.filter((link) => {
    const statusMatches =
      statusFilter.value === 'all' ||
      String(link.status) === statusFilter.value
    const keywordMatches =
      !keyword ||
      link.title.toLowerCase().includes(keyword) ||
      link.slug.toLowerCase().includes(keyword) ||
      link.destination_url.toLowerCase().includes(keyword)
    return statusMatches && keywordMatches
  })
})

onMounted(load)

// copyLink 复制短链地址并显示反馈
async function copyLink(link: Link): Promise<void> {
  try {
    await navigator.clipboard.writeText(link.short_url)
    notice.value = `已复制 /${link.slug}`
  } catch {
    notice.value = '浏览器未允许复制，请手动选择地址'
  }
}

// remove 删除短链并更新本地列表
async function remove(link: Link): Promise<void> {
  if (!window.confirm(`删除 /${link.slug}？删除后该短链将停止跳转。`)) {
    return
  }
  try {
    await deleteLink(link.id)
    removeLink(link.id)
    notice.value = `已删除 /${link.slug}`
  } catch (error) {
    notice.value = error instanceof ApiError ? error.message : '删除失败'
  }
}

// openEdit 打开编辑对话框
function openEdit(link: Link): void {
  editingLink.value = link
}

// handleSaved 更新编辑后的本地链接
function handleSaved(link: Link): void {
  replaceLink(link)
  editingLink.value = null
  notice.value = '链接已更新'
}

// handleCreated 新建后重新加载第一页
async function handleCreated(): Promise<void> {
  createOpen.value = false
  await load()
}

// handleBulkImported 在批量成功写入后刷新链接列表
async function handleBulkImported(count: number): Promise<void> {
  notice.value = `批量导入成功创建 ${count} 条短链`
  await load()
}
</script>

<template>
  <div class="page-stack">
    <header class="page-header">
      <div>
        <p class="section-kicker">链接资产</p>
        <h1>短链接</h1>
        <p>管理目标地址、短码、过期时间和跳转状态。</p>
      </div>
      <div class="detail-actions">
        <button class="secondary-button" type="button" @click="bulkOpen = !bulkOpen">
          {{ bulkOpen ? '收起批量导入' : '批量导入' }}
        </button>
        <button class="primary-button" type="button" @click="createOpen = true">
          创建短链接
        </button>
      </div>
    </header>

    <BulkImportPanel v-if="bulkOpen" @imported="handleBulkImported" />

    <section class="panel link-panel">
      <div class="toolbar">
        <label class="search-field">
          <span class="visually-hidden">搜索短链接</span>
          <input
            v-model="query"
            type="search"
            placeholder="搜索标题、短码或目标地址"
          />
        </label>
        <label class="compact-field">
          <span>状态</span>
          <select v-model="statusFilter">
            <option value="all">全部</option>
            <option value="1">启用</option>
            <option value="2">停用</option>
          </select>
        </label>
        <span class="toolbar-count">{{ filteredLinks.length }} 条</span>
      </div>

      <p v-if="notice" class="inline-notice" role="status">{{ notice }}</p>

      <div v-if="loading" class="skeleton-stack" role="status" aria-label="正在加载链接">
        <span v-for="index in 6" :key="index" class="skeleton-row" />
      </div>
      <div v-else-if="errorMessage" class="error-panel" role="alert">
        <p>{{ errorMessage }}</p>
        <button class="secondary-button" type="button" @click="load">重新加载</button>
      </div>
      <div v-else-if="filteredLinks.length === 0" class="empty-state">
        <h2>{{ links.length === 0 ? '还没有短链接' : '没有匹配结果' }}</h2>
        <p>
          {{
            links.length === 0
              ? '从一个目标地址开始，系统会生成安全随机短码。'
              : '调整搜索词或状态筛选。'
          }}
        </p>
      </div>
      <div v-else class="link-table" role="list">
        <article v-for="link in filteredLinks" :key="link.id" class="link-row" role="listitem">
          <span class="link-status" :class="{ 'is-disabled': link.status !== 1 }">
            {{ statusLabel(link.status) }}
          </span>
          <div class="link-primary">
            <RouterLink :to="`/links/${link.id}`" class="link-title">
              {{ link.title || `/${link.slug}` }}
            </RouterLink>
            <span class="link-destination">{{ shortDestination(link.destination_url) }}</span>
          </div>
          <div class="link-code">
            <code>/{{ link.slug }}</code>
            <button class="text-button" type="button" @click="copyLink(link)">复制</button>
          </div>
          <div class="link-meta">
            <span>{{ link.redirect_code }}</span>
            <time>{{ formatDateTime(link.created_at) }}</time>
          </div>
          <div class="row-actions">
            <button class="small-button" type="button" @click="openEdit(link)">编辑</button>
            <a class="small-button" :href="link.short_url" target="_blank" rel="noreferrer">
              打开
            </a>
            <button class="small-button danger-button" type="button" @click="remove(link)">
              删除
            </button>
          </div>
        </article>
      </div>

      <div v-if="hasMore && !loading" class="load-more-row">
        <button class="secondary-button" type="button" :disabled="loadingMore" @click="loadMore">
          {{ loadingMore ? '加载中…' : '加载更多' }}
        </button>
      </div>
    </section>

    <LinkFormDialog :open="createOpen" @close="createOpen = false" @saved="handleCreated" />
    <LinkFormDialog
      :open="Boolean(editingLink)"
      :link="editingLink"
      @close="editingLink = null"
      @saved="handleSaved"
    />
  </div>
</template>
