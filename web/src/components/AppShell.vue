<script setup lang="ts">
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { RouterLink, RouterView, useRouter } from 'vue-router'

import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const { user, workspace } = storeToRefs(auth)
const router = useRouter()

const initials = computed(() => {
  const name = user.value?.display_name?.trim()
  return name ? name.slice(0, 1).toUpperCase() : 'U'
})

const navigation = computed(() => {
  const items = [
    { label: '工作台', to: '/dashboard' },
    { label: '短链接', to: '/links' },
  ]
  if (user.value?.role === 2) {
    items.push(
      { label: '域名', to: '/domains' },
      { label: '开发者', to: '/developer' },
    )
  }
  return items
})

// signOut 清除登录态并返回登录页
async function signOut(): Promise<void> {
  await auth.logout()
  await router.replace({ name: 'login' })
}
</script>

<template>
  <div class="console-shell">
    <aside class="console-rail">
      <div class="brand-block">
        <span class="brand-mark" aria-hidden="true">SL</span>
        <div>
          <p class="brand-name">短链控制台</p>
          <p class="brand-caption">链接运营工作区</p>
        </div>
      </div>

      <nav class="primary-nav" aria-label="主导航">
        <RouterLink
          v-for="item in navigation"
          :key="item.to"
          :to="item.to"
          class="nav-link"
        >
          <span class="nav-dot" aria-hidden="true" />
          {{ item.label }}
        </RouterLink>
      </nav>

      <div class="rail-foot">
        <div class="workspace-chip">
          <span class="avatar" aria-hidden="true">{{ initials }}</span>
          <div class="workspace-copy">
            <strong>{{ user?.display_name }}</strong>
            <span>{{ workspace?.name }}</span>
          </div>
        </div>
        <button class="text-button rail-action" type="button" @click="signOut">
          退出登录
        </button>
      </div>
    </aside>

    <main class="console-main">
      <RouterView />
    </main>
  </div>
</template>
