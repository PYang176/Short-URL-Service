import { computed, shallowRef } from 'vue'
import { defineStore } from 'pinia'

import {
  getProfile,
  login as loginRequest,
  logout as logoutRequest,
  register as registerRequest,
} from '@/api'
import { clearTokens, getRefreshToken, saveTokens } from '@/api/token-storage'
import type { AuthResponse, User, Workspace } from '@/api/types'

// useAuthStore 管理登录凭证和当前工作空间
export const useAuthStore = defineStore('auth', () => {
  const user = shallowRef<User | null>(null)
  const workspace = shallowRef<Workspace | null>(null)
  const restoring = shallowRef(false)
  const isAuthenticated = computed(() => Boolean(user.value && workspace.value))

  // applyAuth 保存认证响应并同步 Token
  function applyAuth(result: AuthResponse): void {
    user.value = result.user
    workspace.value = result.workspace
    saveTokens(result.token.access_token, result.token.refresh_token)
  }

  // login 登录后保存用户与工作空间
  async function login(email: string, password: string): Promise<void> {
    applyAuth(await loginRequest(email, password))
  }

  // register 注册后直接进入登录状态
  async function register(
    email: string,
    password: string,
    displayName: string,
  ): Promise<void> {
    applyAuth(await registerRequest(email, password, displayName))
  }

  // restoreSession 页面刷新后恢复当前用户
  async function restoreSession(): Promise<void> {
    if (!getRefreshToken()) {
      clearTokens()
      return
    }
    restoring.value = true
    try {
      const profile = await getProfile()
      user.value = profile.user
      workspace.value = profile.workspace
    } catch {
      clearLocalSession()
    } finally {
      restoring.value = false
    }
  }

  // logout 尝试撤销 Refresh Token，并在本地立即退出
  async function logout(): Promise<void> {
    const refreshToken = getRefreshToken()
    try {
      if (refreshToken) {
        await logoutRequest(refreshToken)
      }
    } finally {
      clearLocalSession()
    }
  }

  // clearLocalSession 清除本地身份状态
  function clearLocalSession(): void {
    user.value = null
    workspace.value = null
    clearTokens()
  }

  return {
    user,
    workspace,
    restoring,
    isAuthenticated,
    login,
    register,
    restoreSession,
    logout,
    clearLocalSession,
  }
})
