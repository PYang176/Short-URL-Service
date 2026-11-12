import { computed, shallowRef } from 'vue'
import { defineStore } from 'pinia'

import {
  claimTrialLinks,
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
  const trialClaimNotice = shallowRef('')
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
    trialClaimNotice.value = ''
    try {
      const claim = await claimTrialLinks()
      if (claim.claimed_links > 0) {
        trialClaimNotice.value = `已认领 ${claim.claimed_links} 条试用短链，点击数据也已保留。`
      }
    } catch {
      trialClaimNotice.value =
        '账号已创建，但试用短链未能认领。请重新登录后联系管理员处理。'
    }
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
    trialClaimNotice.value = ''
    clearTokens()
  }

  // consumeTrialClaimNotice 读取一次认领提示并从全局状态中移除
  function consumeTrialClaimNotice(): string {
    const notice = trialClaimNotice.value
    trialClaimNotice.value = ''
    return notice
  }

  return {
    user,
    workspace,
    restoring,
    trialClaimNotice,
    isAuthenticated,
    login,
    register,
    restoreSession,
    logout,
    clearLocalSession,
    consumeTrialClaimNotice,
  }
})
