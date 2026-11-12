<script setup lang="ts">
import { computed, onMounted, shallowRef } from 'vue'
import { storeToRefs } from 'pinia'
import { RouterLink } from 'vue-router'

import LinkFormDialog from '@/components/LinkFormDialog.vue'
import { useLinks } from '@/composables/useLinks'
import { useAuthStore } from '@/stores/auth'
import { formatDateTime, shortDestination, statusLabel } from '@/utils/format'

const auth = useAuthStore()
const { user, workspace } = storeToRefs(auth)
const { links, loading, errorMessage, load } = useLinks(8)
const createOpen = shallowRef(false)
const claimNotice = shallowRef(auth.consumeTrialClaimNotice())

const activeCount = computed(
  () => links.value.filter((link) => link.status === 1).length,
)
const recentLinks = computed(() => links.value.slice(0, 5))

onMounted(load)

// handleCreated 创建后刷新工作台链接
async function handleCreated(): Promise<void> {
  createOpen.value = false
  await load()
}
</script>

<template>
  <div class="page-stack">
    <header class="dashboard-hero">
      <div class="hero-copy">
        <p class="section-kicker">{{ workspace?.name }}</p>
        <h1>{{ user?.display_name }}，今天要发送什么链接？</h1>
        <p class="hero-description">
          创建可追踪的短链接，随后在详情页查看点击趋势、来源设备和二维码。
        </p>
        <button class="primary-button" type="button" @click="createOpen = true">
          创建短链接
        </button>
      </div>

      <div class="route-inspector" aria-label="短链接处理流程">
        <div class="route-track">
          <span class="route-node route-node-source" />
          <span class="route-line" />
          <span class="route-node route-node-code">/{slug}</span>
          <span class="route-line" />
          <span class="route-node route-node-target" />
        </div>
        <div class="route-labels">
          <span>源地址</span>
          <span>缓存跳转</span>
          <span>访问分析</span>
        </div>
      </div>
    </header>

    <p v-if="claimNotice" class="inline-notice" role="status">{{ claimNotice }}</p>

    <section class="metric-strip" aria-label="当前已加载链接概况">
      <div class="metric-cell">
        <span>最近链接</span>
        <strong>{{ links.length }}</strong>
      </div>
      <div class="metric-cell">
        <span>其中启用</span>
        <strong>{{ activeCount }}</strong>
      </div>
      <div class="metric-cell metric-cell-wide">
        <span>默认域名</span>
        <strong class="metric-domain">{{ links[0]?.short_url.split('/r/')[0] ?? '初始化中' }}</strong>
      </div>
    </section>

    <section class="panel">
      <div class="section-heading-row">
        <div>
          <h2>最近创建</h2>
          <p>快速回到最近使用的跳转入口。</p>
        </div>
        <RouterLink class="text-link" to="/links">查看全部短链接</RouterLink>
      </div>

      <div v-if="loading" class="skeleton-stack" role="status" aria-label="正在加载链接">
        <span v-for="index in 4" :key="index" class="skeleton-row" />
      </div>
      <div v-else-if="errorMessage" class="error-panel" role="alert">
        <p>{{ errorMessage }}</p>
        <button class="secondary-button" type="button" @click="load">重新加载</button>
      </div>
      <div v-else-if="recentLinks.length === 0" class="empty-state">
        <h3>还没有短链接</h3>
        <p>创建第一个链接后，这里会显示最近使用的入口。</p>
        <button class="primary-button" type="button" @click="createOpen = true">
          创建第一条短链
        </button>
      </div>
      <div v-else class="recent-list">
        <RouterLink
          v-for="link in recentLinks"
          :key="link.id"
          :to="`/links/${link.id}`"
          class="recent-row"
        >
          <span class="recent-mark" :class="{ 'is-disabled': link.status !== 1 }" />
          <span class="recent-main">
            <strong>{{ link.title || `/${link.slug}` }}</strong>
            <span>{{ shortDestination(link.destination_url) }}</span>
          </span>
          <span class="recent-code">/{{ link.slug }}</span>
          <span class="status-text">{{ statusLabel(link.status) }}</span>
          <time>{{ formatDateTime(link.created_at) }}</time>
        </RouterLink>
      </div>
    </section>

    <LinkFormDialog
      :open="createOpen"
      @close="createOpen = false"
      @saved="handleCreated"
    />
  </div>
</template>
