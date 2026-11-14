<script setup lang="ts">
import { reactive, shallowRef } from 'vue'
import { RouterLink } from 'vue-router'

import { reportAbuse } from '@/api'
import { ApiError } from '@/api/http'

const form = reactive({
  shortURL: '',
  reason: 'phishing',
  details: '',
})
const submitting = shallowRef(false)
const errorMessage = shallowRef('')
const reportID = shallowRef('')

// submit 提交公开举报并只在成功后保留举报编号
async function submit(): Promise<void> {
  if (!form.shortURL.trim() || submitting.value) {
    return
  }
  submitting.value = true
  errorMessage.value = ''
  reportID.value = ''
  try {
    const result = await reportAbuse(
      form.shortURL.trim(),
      form.reason,
      form.details.trim(),
    )
    reportID.value = result.id
    form.shortURL = ''
    form.details = ''
  } catch (error) {
    errorMessage.value =
      error instanceof ApiError ? error.message : '提交举报失败，请稍后重试'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <main class="auth-page">
    <section class="auth-story auth-story-compact">
      <RouterLink class="auth-brand" to="/try">短链控制台</RouterLink>
      <div class="auth-copy">
        <p class="section-kicker">站点安全</p>
        <h1>举报有风险的短链。</h1>
        <p>
          如果短链指向钓鱼、恶意软件、垃圾推广或仿冒页面，请提交给管理员审核。
          管理员确认后可立即下架链接。
        </p>
      </div>
      <RouterLink class="text-link" to="/try">返回短链工具</RouterLink>
    </section>

    <section class="auth-form-wrap">
      <form
        v-if="!reportID"
        class="auth-form"
        aria-labelledby="report-title"
        @submit.prevent="submit"
      >
        <div>
          <p class="section-kicker">公开举报</p>
          <h2 id="report-title">提交举报</h2>
          <p class="auth-switch">无需登录，同一 IP 的提交频率会受到限制。</p>
        </div>

        <p v-if="errorMessage" class="form-error" role="alert">{{ errorMessage }}</p>

        <label class="field">
          <span>可疑短链</span>
          <input
            v-model="form.shortURL"
            type="text"
            maxlength="2048"
            placeholder="例如：https://short.example.com/r/abcd"
            required
          />
        </label>

        <label class="field">
          <span>风险类型</span>
          <select v-model="form.reason">
            <option value="phishing">钓鱼欺诈</option>
            <option value="malware">恶意软件</option>
            <option value="spam">垃圾推广</option>
            <option value="impersonation">仿冒身份</option>
            <option value="other">其他违规</option>
          </select>
        </label>

        <label class="field">
          <span>补充说明，可选</span>
          <textarea
            v-model="form.details"
            rows="4"
            maxlength="1000"
            placeholder="说明页面内容、诱导方式或相关证据。"
          />
        </label>

        <button class="primary-button auth-submit" type="submit" :disabled="submitting">
          {{ submitting ? '正在提交…' : '提交举报' }}
        </button>
      </form>

      <section v-else class="auth-form" role="status">
        <p class="section-kicker">已受理</p>
        <h2>举报已提交</h2>
        <p>举报编号：<code>{{ reportID }}</code></p>
        <p class="auth-switch">管理员审核后会决定是否下架该短链。</p>
        <button class="secondary-button auth-submit" type="button" @click="reportID = ''">
          继续举报其他短链
        </button>
        <RouterLink class="primary-button auth-submit" to="/try">
          返回短链工具
        </RouterLink>
      </section>
    </section>
  </main>
</template>
