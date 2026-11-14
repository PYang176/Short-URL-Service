<script setup lang="ts">
import { computed, nextTick, reactive, shallowRef, useTemplateRef } from 'vue'
import { RouterLink } from 'vue-router'

import { createLink } from '@/api'
import { ApiError } from '@/api/http'
import type { CreateLinkInput, Link } from '@/api/types'
import { formatDateTime } from '@/utils/format'

const emit = defineEmits<{
  created: [link: Link]
}>()

const root = useTemplateRef<HTMLElement>('root')
const destinationInput = useTemplateRef<HTMLInputElement>('destinationInput')
const pending = shallowRef(false)
const errorMessage = shallowRef('')
const notice = shallowRef('')
const latestLink = shallowRef<Link | null>(null)
const idempotencyKey = shallowRef(crypto.randomUUID())
const form = reactive({
  destinationURL: '',
  alias: '',
  expiryHours: 0,
})

const expiryOptions = [
  { value: 0, label: '永不过期' },
  { value: 24, label: '1 天' },
  { value: 168, label: '7 天' },
  { value: 720, label: '30 天' },
] as const

const hasResult = computed(() => latestLink.value !== null)

// focusDestination 滚动到快速创建卡片并聚焦目标地址输入框。
async function focusDestination(): Promise<void> {
  root.value?.scrollIntoView({ behavior: 'smooth', block: 'center' })
  await nextTick()
  destinationInput.value?.focus()
}

// submit 使用默认域名创建一条短链并刷新工作台。
async function submit(): Promise<void> {
  if (pending.value) {
    return
  }
  pending.value = true
  errorMessage.value = ''
  notice.value = ''
  try {
    const input: CreateLinkInput = {
      destination_url: form.destinationURL.trim(),
      alias: form.alias.trim() || undefined,
      title: '',
      notes: '',
      redirect_code: 302,
      expires_at: buildExpiresAt(),
    }
    const created = await createLink(input, idempotencyKey.value)
    latestLink.value = created
    notice.value = ''
    form.destinationURL = ''
    form.alias = ''
    idempotencyKey.value = crypto.randomUUID()
    emit('created', created)
  } catch (error) {
    errorMessage.value =
      error instanceof ApiError ? error.message : '创建短链失败，请稍后重试'
  } finally {
    pending.value = false
  }
}

// buildExpiresAt 将快速创建的有效期选项转换为 UTC RFC3339。
function buildExpiresAt(): string | undefined {
  if (form.expiryHours <= 0) {
    return undefined
  }
  return new Date(Date.now() + form.expiryHours * 60 * 60 * 1000).toISOString()
}

// copyLatest 复制刚创建完成的短链。
async function copyLatest(): Promise<void> {
  if (!latestLink.value) {
    return
  }
  try {
    await navigator.clipboard.writeText(latestLink.value.short_url)
    notice.value = '已复制短链接'
  } catch {
    notice.value = '浏览器未允许复制，请手动选择地址'
  }
}

defineExpose({ focusDestination })
</script>

<template>
  <section ref="root" class="quick-create-panel" aria-labelledby="quick-create-title">
    <div class="quick-create-heading">
      <div class="quick-create-icon" aria-hidden="true">↗</div>
      <div>
        <h2 id="quick-create-title">新建短链接</h2>
        <p>粘贴目标地址，短码和有效期可按需调整。</p>
      </div>
    </div>

    <form class="quick-create-form" @submit.prevent="submit">
      <label class="quick-target-field">
        <span class="visually-hidden">目标地址</span>
        <input
          ref="destinationInput"
          v-model="form.destinationURL"
          name="destination_url"
          type="url"
          inputmode="url"
          autocomplete="url"
          placeholder="在此粘贴需要缩短的长链接，例如 https://example.com/..."
          maxlength="2048"
          required
        />
      </label>
      <p class="quick-create-hint">支持 HTTP / HTTPS，地址格式会在创建时自动校验。</p>

      <div class="quick-create-actions">
        <label class="quick-inline-field">
          <span class="visually-hidden">自定义短码</span>
          <input
            v-model="form.alias"
            name="alias"
            type="text"
            minlength="4"
            maxlength="32"
            pattern="[A-Za-z0-9_\-]{4,32}"
            placeholder="自定义短码，可选"
          />
        </label>
        <label class="quick-inline-field quick-expiry-field">
          <span class="visually-hidden">链接有效期</span>
          <select v-model="form.expiryHours">
            <option v-for="option in expiryOptions" :key="option.value" :value="option.value">
              {{ option.label }}
            </option>
          </select>
        </label>
        <button class="primary-button quick-create-submit" type="submit" :disabled="pending">
          {{ pending ? '正在生成…' : '生成短链接' }}
        </button>
      </div>

      <p v-if="errorMessage" class="form-error" role="alert">{{ errorMessage }}</p>
      <p v-if="notice" class="inline-notice" role="status">{{ notice }}</p>
    </form>

    <section v-if="hasResult && latestLink" class="quick-result" aria-label="刚创建的短链接">
      <div class="quick-result-copy">
        <span>短链接已生成</span>
        <a :href="latestLink.short_url" target="_blank" rel="noreferrer">
          {{ latestLink.short_url }}
        </a>
        <small v-if="latestLink.expires_at">
          失效时间：{{ formatDateTime(latestLink.expires_at) }}
        </small>
      </div>
      <div class="quick-result-actions">
        <button class="secondary-button" type="button" @click="copyLatest">复制</button>
        <RouterLink class="text-link" :to="`/links/${latestLink.id}`">查看详情</RouterLink>
      </div>
    </section>
  </section>
</template>

<style scoped>
.quick-create-panel {
  display: grid;
  gap: 1rem;
  padding: clamp(1.1rem, 2.5vw, 1.55rem);
  border: 1px solid var(--line);
  border-radius: var(--radius-lg);
  background: var(--surface-strong);
  box-shadow: 0 16px 40px rgba(31, 70, 64, 0.07);
}

.quick-create-heading {
  display: flex;
  align-items: center;
  gap: 0.7rem;
}

.quick-create-icon {
  display: grid;
  width: 2rem;
  height: 2rem;
  place-items: center;
  border-radius: 50%;
  background: var(--accent-soft);
  color: var(--accent-strong);
  font-size: 1rem;
  font-weight: 800;
}

.quick-create-heading h2 {
  margin: 0;
  font-family: var(--font-body);
  font-size: 1.1rem;
  letter-spacing: -0.02em;
}

.quick-create-heading p {
  margin: 0.2rem 0 0;
  color: var(--muted);
  font-size: 0.76rem;
}

.quick-create-form {
  display: grid;
  gap: 0.7rem;
}

.quick-target-field input,
.quick-inline-field input,
.quick-inline-field select {
  width: 100%;
  min-height: 2.85rem;
  padding: 0.65rem 0.8rem;
  border: 1px solid var(--line-strong);
  border-radius: var(--radius-sm);
  background: #fbfdfc;
  color: var(--ink);
  outline: none;
}

.quick-target-field input {
  min-height: 3.2rem;
  font-family: var(--font-mono);
  font-size: 0.86rem;
}

.quick-inline-field input:focus,
.quick-inline-field select:focus,
.quick-target-field input:focus {
  border-color: var(--accent);
  box-shadow: 0 0 0 3px rgba(22, 124, 108, 0.11);
}

.quick-create-hint {
  margin: -0.15rem 0 0;
  color: var(--muted);
  font-size: 0.72rem;
}

.quick-create-actions {
  display: grid;
  grid-template-columns: minmax(10rem, 1fr) minmax(9rem, 0.38fr) auto;
  gap: 0.6rem;
}

.quick-create-submit {
  min-width: 8.5rem;
}

.quick-result {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  padding: 0.85rem 0.95rem;
  border: 1px solid #a8d7cc;
  border-radius: var(--radius-md);
  background: var(--accent-soft);
}

.quick-result-copy {
  display: grid;
  min-width: 0;
}

.quick-result-copy > span {
  color: var(--accent-strong);
  font-size: 0.7rem;
  font-weight: 750;
}

.quick-result-copy a {
  overflow: hidden;
  color: var(--accent-strong);
  font: 650 0.82rem/1.5 var(--font-mono);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.quick-result-copy small {
  color: var(--muted);
  font-size: 0.68rem;
}

.quick-result-actions {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 0.75rem;
}

@media (max-width: 760px) {
  .quick-create-actions {
    grid-template-columns: 1fr 1fr;
  }

  .quick-create-submit {
    grid-column: 1 / -1;
    width: 100%;
  }
}

@media (max-width: 520px) {
  .quick-create-actions {
    grid-template-columns: 1fr;
  }

  .quick-create-submit {
    grid-column: auto;
  }

  .quick-result {
    align-items: stretch;
    flex-direction: column;
  }

  .quick-result-actions {
    justify-content: space-between;
  }
}
</style>
