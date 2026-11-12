<script setup lang="ts">
import { shallowRef } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'

import { ApiError } from '@/api/http'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()
const email = shallowRef('')
const password = shallowRef('')
const pending = shallowRef(false)
const errorMessage = shallowRef(
  route.query.expired === '1' ? '登录状态已失效，请重新登录' : '',
)

// submit 登录并返回原目标页面
async function submit(): Promise<void> {
  pending.value = true
  errorMessage.value = ''
  try {
    await auth.login(email.value.trim(), password.value)
    const redirect =
      typeof route.query.redirect === 'string' ? route.query.redirect : '/dashboard'
    await router.replace(redirect)
  } catch (error) {
    errorMessage.value =
      error instanceof ApiError ? error.message : '登录失败，请稍后重试'
  } finally {
    pending.value = false
  }
}
</script>

<template>
  <main class="auth-page">
    <section class="auth-story" aria-labelledby="auth-title">
      <div class="auth-brand">
        <span class="brand-mark" aria-hidden="true">SL</span>
        <span>短链控制台</span>
      </div>
      <div class="auth-copy">
        <p class="section-kicker">链接运营</p>
        <h1 id="auth-title">看清每一次点击从哪里来。</h1>
        <p>
          在一个工作区里创建短链、下载二维码并检查访问趋势。跳转保持简单，
          数据保持可读。
        </p>
      </div>
      <ol class="route-story" aria-label="短链处理流程">
        <li><span>01</span>创建目标与短码</li>
        <li><span>02</span>缓存并完成跳转</li>
        <li><span>03</span>异步记录访问</li>
      </ol>
    </section>

    <section class="auth-form-wrap">
      <form class="auth-form" @submit.prevent="submit">
        <div>
          <p class="section-kicker">欢迎回来</p>
          <h2>登录工作空间</h2>
        </div>

        <label class="field">
          <span>邮箱</span>
          <input
            v-model="email"
            name="email"
            type="email"
            autocomplete="email"
            placeholder="you@example.com"
            required
            autofocus
          />
        </label>

        <label class="field">
          <span>密码</span>
          <input
            v-model="password"
            name="password"
            type="password"
            autocomplete="current-password"
            minlength="8"
            required
          />
        </label>

        <p v-if="errorMessage" class="form-error" role="alert">{{ errorMessage }}</p>

        <button class="primary-button auth-submit" type="submit" :disabled="pending">
          {{ pending ? '正在登录…' : '登录' }}
        </button>

        <p class="auth-switch">
          还没有账号？
          <RouterLink to="/register">创建账号</RouterLink>
        </p>
      </form>
    </section>
  </main>
</template>
