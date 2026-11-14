<script setup lang="ts">
import { computed } from 'vue'
import { RouterView, useRoute, useRouter } from 'vue-router'

import AuthDialog from '@/components/AuthDialog.vue'

type AuthMode = 'login' | 'register'

const route = useRoute()
const router = useRouter()
const authMode = computed<AuthMode>(() =>
  route.query.auth === 'register' ? 'register' : 'login',
)
const authOpen = computed(
  () => route.query.auth === 'login' || route.query.auth === 'register',
)
const authNotice = computed(() =>
  route.query.expired === '1' ? '登录状态已失效，请重新登录。' : '',
)

// closeAuth 移除认证查询参数，保留用户当前所在页面。
async function closeAuth(): Promise<void> {
  await router.replace({
    query: {
      ...route.query,
      auth: undefined,
      expired: undefined,
      redirect: undefined,
    },
  })
}

// switchAuthMode 通过 URL 状态同步登录与注册标签页。
async function switchAuthMode(mode: AuthMode): Promise<void> {
  await router.replace({
    query: {
      ...route.query,
      auth: mode,
      expired: undefined,
    },
  })
}

// completeAuth 登录后返回原目标页面，默认进入工作台。
async function completeAuth(): Promise<void> {
  const redirect = getSafeRedirect(route.query.redirect)
  await router.replace(redirect)
}

// getSafeRedirect 拒绝外部地址，只允许站内绝对路径。
function getSafeRedirect(value: unknown): string {
  return typeof value === 'string' &&
    value.startsWith('/') &&
    !value.startsWith('//')
    ? value
    : '/dashboard'
}
</script>

<template>
  <RouterView />
  <AuthDialog
    :open="authOpen"
    :mode="authMode"
    :initial-notice="authNotice"
    @close="closeAuth"
    @update:mode="switchAuthMode"
    @authenticated="completeAuth"
  />
</template>
