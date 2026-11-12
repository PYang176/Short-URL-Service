<script setup lang="ts">
import { shallowRef } from 'vue'
import { RouterLink, useRouter } from 'vue-router'

import { ApiError } from '@/api/http'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const router = useRouter()
const displayName = shallowRef('')
const email = shallowRef('')
const password = shallowRef('')
const confirmPassword = shallowRef('')
const pending = shallowRef(false)
const errorMessage = shallowRef('')

// submit 创建账号并进入工作台
async function submit(): Promise<void> {
  if (password.value !== confirmPassword.value) {
    errorMessage.value = '两次输入的密码不一致'
    return
  }
  pending.value = true
  errorMessage.value = ''
  try {
    await auth.register(email.value.trim(), password.value, displayName.value.trim())
    await router.replace('/dashboard')
  } catch (error) {
    errorMessage.value =
      error instanceof ApiError ? error.message : '注册失败，请稍后重试'
  } finally {
    pending.value = false
  }
}
</script>

<template>
  <main class="auth-page">
    <section class="auth-story auth-story-compact" aria-labelledby="register-story-title">
      <div class="auth-brand">
        <span class="brand-mark" aria-hidden="true">SL</span>
        <span>短链控制台</span>
      </div>
      <div class="auth-copy">
        <p class="section-kicker">新工作空间</p>
        <h1 id="register-story-title">从第一条链接开始建立自己的访问视图。</h1>
        <p>注册后会自动创建个人工作空间，链接和分析数据彼此隔离。</p>
      </div>
      <div class="registration-note">
        当前版本使用本地 MySQL 和 Redis，适合个人项目与小型团队。
      </div>
    </section>

    <section class="auth-form-wrap">
      <form class="auth-form" @submit.prevent="submit">
        <div>
          <p class="section-kicker">创建账号</p>
          <h2>进入控制台</h2>
        </div>

        <label class="field">
          <span>显示名称</span>
          <input
            v-model="displayName"
            name="display_name"
            type="text"
            maxlength="64"
            autocomplete="nickname"
            required
            autofocus
          />
        </label>

        <label class="field">
          <span>邮箱</span>
          <input
            v-model="email"
            name="email"
            type="email"
            autocomplete="email"
            required
          />
        </label>

        <label class="field">
          <span>密码</span>
          <input
            v-model="password"
            name="password"
            type="password"
            autocomplete="new-password"
            minlength="8"
            required
          />
        </label>

        <label class="field">
          <span>确认密码</span>
          <input
            v-model="confirmPassword"
            name="confirm_password"
            type="password"
            autocomplete="new-password"
            minlength="8"
            required
          />
        </label>

        <p v-if="errorMessage" class="form-error" role="alert">{{ errorMessage }}</p>

        <button class="primary-button auth-submit" type="submit" :disabled="pending">
          {{ pending ? '正在创建…' : '创建账号' }}
        </button>

        <p class="auth-switch">
          已有账号？
          <RouterLink to="/login">返回登录</RouterLink>
        </p>
      </form>
    </section>
  </main>
</template>
