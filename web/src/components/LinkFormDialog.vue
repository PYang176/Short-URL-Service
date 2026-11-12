<script setup lang="ts">
import { reactive, shallowRef, useTemplateRef, watch } from 'vue'

import { createLink, listDomains, updateLink } from '@/api'
import { ApiError } from '@/api/http'
import type { CreateLinkInput, Domain, Link, UpdateLinkInput } from '@/api/types'

const props = defineProps<{
  open: boolean
  link?: Link | null
}>()

const emit = defineEmits<{
  close: []
  saved: [link: Link]
}>()

const dialog = useTemplateRef<HTMLDialogElement>('dialog')
const pending = shallowRef(false)
const errorMessage = shallowRef('')
const domains = shallowRef<Domain[]>([])
const idempotencyKey = shallowRef('')
const form = reactive({
  destinationURL: '',
  domainID: '',
  alias: '',
  title: '',
  notes: '',
  redirectCode: 302,
  status: 1,
  startsAt: '',
  expiresAt: '',
})

const editing = () => Boolean(props.link)

// resetForm 使用当前链接或空值重置表单
function resetForm(): void {
  const link = props.link
  form.destinationURL = link?.destination_url ?? ''
  form.domainID = link?.domain_id ?? ''
  form.alias = link?.slug ?? ''
  form.title = link?.title ?? ''
  form.notes = link?.notes ?? ''
  form.redirectCode = link?.redirect_code ?? 302
  form.status = link?.status ?? 1
  form.startsAt = toLocalDateTime(link?.starts_at ?? null)
  form.expiresAt = toLocalDateTime(link?.expires_at ?? null)
  idempotencyKey.value = link ? '' : `web-${crypto.randomUUID()}`
  errorMessage.value = ''
}

// submit 根据新建或编辑状态提交短链
async function submit(): Promise<void> {
  if (pending.value) {
    return
  }
  pending.value = true
  errorMessage.value = ''
  try {
    const saved = props.link
      ? await updateLink(props.link.id, buildUpdateInput())
      : await createLink(buildCreateInput(), idempotencyKey.value)
    emit('saved', saved)
    emit('close')
  } catch (error) {
    errorMessage.value =
      error instanceof ApiError ? error.message : '保存失败，请稍后重试'
  } finally {
    pending.value = false
  }
}

// buildCreateInput 转换新建短链表单
function buildCreateInput(): CreateLinkInput {
  return {
    destination_url: form.destinationURL.trim(),
    domain_id: form.domainID || undefined,
    alias: form.alias.trim() || undefined,
    title: form.title.trim(),
    notes: form.notes.trim(),
    redirect_code: form.redirectCode,
    starts_at: toISOString(form.startsAt),
    expires_at: toISOString(form.expiresAt),
  }
}

// buildUpdateInput 转换编辑短链表单
function buildUpdateInput(): UpdateLinkInput {
  return {
    destination_url: form.destinationURL.trim(),
    title: form.title.trim(),
    notes: form.notes.trim(),
    status: form.status,
    redirect_code: form.redirectCode,
    expires_at: toISOString(form.expiresAt),
    clear_expires_at: !form.expiresAt,
  }
}

// closeDialog 通知父组件关闭对话框
function closeDialog(): void {
  if (!pending.value) {
    emit('close')
  }
}

// toLocalDateTime 将 RFC3339 转为 datetime-local 所需格式
function toLocalDateTime(value: string | null): string {
  if (!value) {
    return ''
  }
  const date = new Date(value)
  const offset = date.getTimezoneOffset() * 60_000
  return new Date(date.getTime() - offset).toISOString().slice(0, 16)
}

// toISOString 将 datetime-local 转为 UTC RFC3339
function toISOString(value: string): string | null {
  return value ? new Date(value).toISOString() : null
}

watch(
  () => props.open,
  (open) => {
    if (open) {
      resetForm()
      void loadVerifiedDomains()
      if (!dialog.value?.open) {
        dialog.value?.showModal()
      }
    } else if (dialog.value?.open) {
      dialog.value.close()
    }
  },
  { immediate: true },
)

// loadVerifiedDomains 加载可用于创建短链的已验证域名
async function loadVerifiedDomains(): Promise<void> {
  try {
    const result = await listDomains()
    domains.value = result.items.filter((domain) => domain.verified)
  } catch {
    domains.value = []
  }
}
</script>

<template>
  <dialog
    ref="dialog"
    class="dialog-panel"
    aria-labelledby="link-dialog-title"
    @cancel.prevent="closeDialog"
    @click="($event.target === $event.currentTarget) && closeDialog()"
  >
    <form class="dialog-card" @submit.prevent="submit">
      <header class="dialog-header">
        <div>
          <p class="section-kicker">{{ editing() ? '修改路由' : '建立新路由' }}</p>
          <h2 id="link-dialog-title">{{ editing() ? '编辑短链接' : '创建短链接' }}</h2>
        </div>
        <button
          class="icon-button"
          type="button"
          aria-label="关闭对话框"
          :disabled="pending"
          @click="closeDialog"
        >
          ×
        </button>
      </header>

      <div class="form-grid">
        <label class="field field-wide">
          <span>目标地址</span>
          <input
            v-model="form.destinationURL"
            name="destination_url"
            type="url"
            inputmode="url"
            placeholder="https://example.com/landing"
            required
            autofocus
          />
        </label>

        <label v-if="!editing()" class="field">
          <span>自定义短码</span>
          <input
            v-model="form.alias"
            name="alias"
            type="text"
            minlength="4"
            maxlength="32"
            pattern="[A-Za-z0-9_\-]{4,32}"
            placeholder="留空自动生成"
          />
        </label>

        <label v-if="!editing()" class="field">
          <span>域名</span>
          <select v-model="form.domainID">
            <option value="">平台默认域名</option>
            <option v-for="domain in domains" :key="domain.id" :value="domain.id">
              {{ domain.hostname }}
            </option>
          </select>
        </label>

        <label v-else class="field">
          <span>域名</span>
          <input :value="props.link?.domain_hostname" readonly />
        </label>

        <label class="field">
          <span>标题</span>
          <input
            v-model="form.title"
            name="title"
            type="text"
            maxlength="255"
            placeholder="用于工作台识别"
          />
        </label>

        <label class="field field-wide">
          <span>备注</span>
          <textarea
            v-model="form.notes"
            name="notes"
            rows="3"
            maxlength="1000"
            placeholder="可选，记录活动或投放信息"
          />
        </label>

        <label class="field">
          <span>跳转方式</span>
          <select v-model="form.redirectCode" name="redirect_code">
            <option :value="302">302 临时跳转</option>
            <option :value="301">301 永久跳转</option>
          </select>
        </label>

        <label v-if="editing()" class="field">
          <span>状态</span>
          <select v-model="form.status" name="status">
            <option :value="1">启用</option>
            <option :value="2">停用</option>
          </select>
        </label>

        <label v-if="!editing()" class="field">
          <span>生效时间</span>
          <input v-model="form.startsAt" name="starts_at" type="datetime-local" />
        </label>

        <label class="field">
          <span>过期时间</span>
          <input v-model="form.expiresAt" name="expires_at" type="datetime-local" />
        </label>
      </div>

      <p v-if="errorMessage" class="form-error" role="alert">{{ errorMessage }}</p>

      <footer class="dialog-actions">
        <button class="secondary-button" type="button" :disabled="pending" @click="closeDialog">
          取消
        </button>
        <button class="primary-button" type="submit" :disabled="pending">
          {{ pending ? '保存中…' : editing() ? '保存修改' : '创建短链' }}
        </button>
      </footer>
    </form>
  </dialog>
</template>
