<script setup lang="ts">
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { RouterLink, RouterView, useRouter } from 'vue-router'

import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const { user, workspace } = storeToRefs(auth)
const router = useRouter()

const navigation = computed(() => {
  const items = [
    { label: '工作台', to: '/dashboard' },
    { label: '短链接', to: '/links' },
  ]
  if (user.value?.role === 2) {
    items.push(
      { label: '域名', to: '/domains' },
      { label: '开发者', to: '/developer' },
      { label: '治理', to: '/admin/abuse' },
    )
  }
  return items
})

// openCreate 回到工作台并聚焦快速创建表单。
async function openCreate(): Promise<void> {
  await router.push({ name: 'dashboard', query: { create: '1' } })
}

// signOut 清除登录态并返回免登录工具页。
async function signOut(): Promise<void> {
  await auth.logout()
  await router.replace({ name: 'try' })
}
</script>

<template>
  <div class="workspace-shell">
    <header class="workspace-header">
      <div class="workspace-header-inner">
        <RouterLink class="workspace-brand" to="/dashboard" aria-label="返回工作台">
          <span class="brand-mark" aria-hidden="true">SL</span>
          <span class="workspace-brand-copy">
            <strong>短链控制台</strong>
            <small>{{ workspace?.name }}</small>
          </span>
        </RouterLink>

        <nav class="primary-nav workspace-nav" aria-label="主导航">
          <RouterLink
            v-for="item in navigation"
            :key="item.to"
            :to="item.to"
            class="nav-link workspace-nav-link"
          >
            {{ item.label }}
          </RouterLink>
        </nav>

        <div class="workspace-actions">
          <button class="workspace-create-button" type="button" @click="openCreate">
            <span aria-hidden="true">+</span>
            新建短链接
          </button>
          <span class="account-chip" :title="user?.email">{{ user?.email }}</span>
          <button
            class="logout-button"
            type="button"
            aria-label="退出登录"
            title="退出登录"
            @click="signOut"
          >
            ↪
          </button>
        </div>
      </div>
    </header>

    <main class="workspace-main">
      <RouterView />
    </main>
  </div>
</template>

<style scoped>
.workspace-shell {
  min-height: 100vh;
  background:
    radial-gradient(circle at 78% -8rem, rgba(22, 124, 108, 0.08), transparent 30rem),
    var(--surface);
}

.workspace-header {
  position: sticky;
  z-index: 20;
  top: 0;
  border-bottom: 1px solid rgba(185, 201, 197, 0.72);
  background: rgba(250, 252, 251, 0.92);
  backdrop-filter: blur(16px);
}

.workspace-header-inner {
  display: flex;
  width: min(82rem, calc(100% - 2rem));
  min-height: 4.6rem;
  align-items: center;
  gap: 1.5rem;
  margin: 0 auto;
}

.workspace-brand {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 0.65rem;
  text-decoration: none;
}

.workspace-brand .brand-mark {
  border-color: var(--line-strong);
  background: var(--accent-soft);
}

.workspace-brand-copy {
  display: grid;
  line-height: 1.25;
}

.workspace-brand-copy strong {
  font-size: 0.9rem;
}

.workspace-brand-copy small {
  max-width: 10rem;
  overflow: hidden;
  color: var(--muted);
  font-size: 0.64rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.primary-nav.workspace-nav {
  display: flex;
  min-width: 0;
  flex: 1;
  gap: 0.2rem;
  justify-content: flex-start;
}

.workspace-nav-link.nav-link {
  padding: 0.58rem 0.72rem;
  border: 1px solid transparent;
  border-radius: var(--radius-sm);
  color: var(--muted-strong);
  font-size: 0.8rem;
  font-weight: 650;
}

.workspace-nav-link.nav-link:hover {
  border-color: var(--line);
  background: var(--surface-strong);
  color: var(--ink);
}

.workspace-nav-link.nav-link.router-link-active {
  border-color: var(--line);
  background: var(--surface-strong);
  color: var(--accent-strong);
  box-shadow: 0 3px 12px rgba(31, 71, 64, 0.07);
}

.workspace-actions {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 0.5rem;
}

.workspace-create-button {
  display: inline-flex;
  min-height: 2.35rem;
  align-items: center;
  gap: 0.35rem;
  padding: 0.48rem 0.75rem;
  border: 0;
  border-radius: var(--radius-sm);
  background: var(--accent);
  color: #ffffff;
  font-size: 0.78rem;
  font-weight: 750;
}

.workspace-create-button:hover {
  background: var(--accent-strong);
}

.account-chip {
  max-width: 14rem;
  overflow: hidden;
  padding: 0.48rem 0.7rem;
  border: 1px solid #b9dfd7;
  border-radius: 999px;
  background: #e8f5f1;
  color: var(--accent-strong);
  font-size: 0.72rem;
  font-weight: 650;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.logout-button {
  display: grid;
  width: 2.2rem;
  height: 2.2rem;
  place-items: center;
  border: 1px solid var(--line);
  border-radius: 50%;
  background: var(--surface-strong);
  color: var(--muted-strong);
  font-size: 1rem;
}

.logout-button:hover {
  border-color: var(--accent);
  color: var(--accent-strong);
}

.workspace-main {
  min-width: 0;
}

@media (max-width: 900px) {
  .workspace-header-inner {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    grid-template-rows: auto auto;
    gap: 0.65rem;
    padding: 0.7rem 0;
  }

  .workspace-brand {
    grid-column: 1;
    grid-row: 1;
  }

  .primary-nav.workspace-nav {
    grid-column: 1 / -1;
    grid-row: 2;
    width: 100%;
    overflow-x: auto;
    padding-bottom: 0.1rem;
    scrollbar-width: none;
  }

  .primary-nav.workspace-nav::-webkit-scrollbar {
    display: none;
  }

  .workspace-nav-link.nav-link {
    flex: 0 0 auto;
  }

  .workspace-actions {
    grid-column: 2;
    grid-row: 1;
    margin-left: auto;
  }
}

@media (max-width: 620px) {
  .workspace-header-inner {
    width: calc(100% - 1.25rem);
  }

  .workspace-brand-copy small,
  .account-chip {
    display: none;
  }

  .workspace-create-button {
    padding-inline: 0.62rem;
  }
}
</style>
