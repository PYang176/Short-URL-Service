<script setup lang="ts">
import {
  nextTick,
  onBeforeUnmount,
  reactive,
  shallowRef,
  useTemplateRef,
  watch,
} from 'vue'

import { ApiError } from '@/api/http'
import { useAuthStore } from '@/stores/auth'

type AuthMode = 'login' | 'register'

const props = defineProps<{
  open: boolean
  mode: AuthMode
  initialNotice?: string
}>()

const emit = defineEmits<{
  close: []
  'update:mode': [mode: AuthMode]
  authenticated: []
}>()

const auth = useAuthStore()
const dialog = useTemplateRef<HTMLDialogElement>('dialog')
const displayNameInput = useTemplateRef<HTMLInputElement>('displayNameInput')
const emailInput = useTemplateRef<HTMLInputElement>('emailInput')
const pending = shallowRef(false)
const errorMessage = shallowRef('')
const showPassword = shallowRef(false)
const form = reactive({
  displayName: '',
  email: '',
  password: '',
  confirmPassword: '',
})

let previousBodyOverflow = ''

// switchMode 在登录和注册表单之间切换，并清理一次性错误状态。
function switchMode(mode: AuthMode): void {
  if (pending.value || props.mode === mode) {
    return
  }
  errorMessage.value = ''
  form.password = ''
  form.confirmPassword = ''
  showPassword.value = false
  emit('update:mode', mode)
}

// submit 根据当前模式完成登录或注册，并把成功交给全局路由协调。
async function submit(): Promise<void> {
  if (pending.value) {
    return
  }
  errorMessage.value = ''
  if (props.mode === 'register' && form.password !== form.confirmPassword) {
    errorMessage.value = '两次输入的密码不一致'
    return
  }

  pending.value = true
  try {
    if (props.mode === 'login') {
      await auth.login(form.email.trim(), form.password)
    } else {
      await auth.register(
        form.email.trim(),
        form.password,
        form.displayName.trim(),
      )
    }
    emit('authenticated')
  } catch (error) {
    errorMessage.value =
      error instanceof ApiError
        ? error.message
        : props.mode === 'login'
          ? '登录失败，请稍后重试'
          : '注册失败，请稍后重试'
  } finally {
    pending.value = false
  }
}

// closeDialog 在非提交状态下关闭认证弹窗。
function closeDialog(): void {
  if (!pending.value) {
    emit('close')
  }
}

// focusCurrentField 根据当前模式聚焦第一个表单字段。
async function focusCurrentField(): Promise<void> {
  await nextTick()
  if (props.mode === 'register') {
    displayNameInput.value?.focus()
  } else {
    emailInput.value?.focus()
  }
}

watch(
  () => props.open,
  async (open) => {
    await nextTick()
    if (!dialog.value) {
      return
    }
    if (open) {
      previousBodyOverflow = document.body.style.overflow
      document.body.style.overflow = 'hidden'
      errorMessage.value = ''
      if (!dialog.value.open) {
        dialog.value.showModal()
      }
      void focusCurrentField()
      return
    }
    form.password = ''
    form.confirmPassword = ''
    showPassword.value = false
    document.body.style.overflow = previousBodyOverflow
    if (dialog.value.open) {
      dialog.value.close()
    }
  },
  { immediate: true },
)

watch(
  () => props.mode,
  () => {
    errorMessage.value = ''
    if (props.open) {
      void focusCurrentField()
    }
  },
)

onBeforeUnmount(() => {
  document.body.style.overflow = previousBodyOverflow
})
</script>

<template>
  <dialog
    ref="dialog"
    class="auth-dialog"
    aria-labelledby="auth-dialog-title"
    @cancel.prevent="closeDialog"
    @click="($event.target === $event.currentTarget) && closeDialog()"
  >
    <section class="auth-card">
      <header class="auth-card-header">
        <div class="auth-dialog-brand">
          <span class="brand-mark" aria-hidden="true">SL</span>
          <span>短链控制台</span>
        </div>

        <div class="auth-header-actions">
          <div class="auth-mode-switch" role="tablist" aria-label="登录或注册">
            <button
              type="button"
              role="tab"
              :aria-selected="mode === 'login'"
              :class="{ 'is-active': mode === 'login' }"
              @click="switchMode('login')"
            >
              登录
            </button>
            <button
              type="button"
              role="tab"
              :aria-selected="mode === 'register'"
              :class="{ 'is-active': mode === 'register' }"
              @click="switchMode('register')"
            >
              注册
            </button>
          </div>
          <button
            class="auth-close-button"
            type="button"
            aria-label="关闭认证窗口"
            :disabled="pending"
            @click="closeDialog"
          >
            ×
          </button>
        </div>
      </header>

      <div class="auth-card-body">
        <div class="auth-dialog-heading">
          <p class="section-kicker">{{ mode === 'login' ? '欢迎回来' : '创建账号' }}</p>
          <h2 id="auth-dialog-title">
            {{ mode === 'login' ? '登录后管理你的短链' : '从第一条短链开始' }}
          </h2>
          <p>
            {{
              mode === 'login'
                ? '登录后会回到刚才的页面，试用链接也可以继续保留。'
                : '注册后会自动创建工作空间，并认领当前浏览器的试用短链。'
            }}
          </p>
        </div>

        <p v-if="initialNotice" class="auth-notice" role="status">
          {{ initialNotice }}
        </p>

        <form class="auth-dialog-form" @submit.prevent="submit">
          <label v-if="mode === 'register'" class="auth-field">
            <span>显示名称</span>
            <input
              ref="displayNameInput"
              v-model="form.displayName"
              name="display_name"
              type="text"
              autocomplete="nickname"
              maxlength="64"
              required
            />
          </label>

          <label class="auth-field">
            <span>邮箱</span>
            <input
              ref="emailInput"
              v-model="form.email"
              name="email"
              type="email"
              autocomplete="email"
              placeholder="you@example.com"
              required
            />
          </label>

          <label class="auth-field">
            <span>密码</span>
            <div class="password-field">
              <input
                v-model="form.password"
                name="password"
                aria-label="密码"
                :type="showPassword ? 'text' : 'password'"
                :autocomplete="mode === 'login' ? 'current-password' : 'new-password'"
                minlength="8"
                required
              />
              <button
                type="button"
                class="password-toggle"
                :aria-label="showPassword ? '隐藏密码' : '显示密码'"
                @click="showPassword = !showPassword"
              >
                {{ showPassword ? '隐藏' : '显示' }}
              </button>
            </div>
          </label>

          <label v-if="mode === 'register'" class="auth-field">
            <span>确认密码</span>
            <input
              v-model="form.confirmPassword"
              name="confirm_password"
              aria-label="确认密码"
              :type="showPassword ? 'text' : 'password'"
              autocomplete="new-password"
              minlength="8"
              required
            />
          </label>

          <p v-if="errorMessage" class="form-error" role="alert">{{ errorMessage }}</p>

          <button class="auth-submit-button" type="submit" :disabled="pending">
            {{
              pending
                ? mode === 'login'
                  ? '正在登录…'
                  : '正在创建…'
                : mode === 'login'
                  ? '登录'
                  : '创建账号'
            }}
          </button>
        </form>

        <p class="auth-dialog-switch">
          {{ mode === 'login' ? '还没有账号？' : '已经有账号？' }}
          <button
            type="button"
            @click="switchMode(mode === 'login' ? 'register' : 'login')"
          >
            {{ mode === 'login' ? '立即注册' : '返回登录' }}
          </button>
        </p>
      </div>
    </section>
  </dialog>
</template>

<style scoped>
.auth-dialog {
  width: min(31rem, calc(100vw - 2rem));
  max-width: none;
  max-height: calc(100vh - 2rem);
  padding: 0;
  border: 0;
  border-radius: 1.2rem;
  background: transparent;
  color: var(--ink);
  overflow: auto;
}

.auth-dialog::backdrop {
  background: rgba(19, 37, 34, 0.42);
  backdrop-filter: blur(8px);
}

.auth-card {
  overflow: hidden;
  border: 1px solid rgba(255, 255, 255, 0.72);
  border-radius: 1.2rem;
  background: #ffffff;
  box-shadow: 0 28px 80px rgba(20, 52, 47, 0.22);
}

.auth-card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  padding: 1.1rem 1.2rem 0;
}

.auth-dialog-brand {
  display: inline-flex;
  align-items: center;
  gap: 0.55rem;
  font-size: 0.82rem;
  font-weight: 750;
}

.auth-dialog-brand .brand-mark {
  width: 2.1rem;
  height: 2.1rem;
  border-color: var(--line-strong);
}

.auth-header-actions {
  display: flex;
  align-items: center;
  gap: 0.55rem;
}

.auth-mode-switch {
  display: grid;
  grid-template-columns: 1fr 1fr;
  padding: 0.22rem;
  border-radius: 0.75rem;
  background: #f0f4f3;
}

.auth-mode-switch button {
  min-width: 3.8rem;
  padding: 0.48rem 0.65rem;
  border: 1px solid transparent;
  border-radius: 0.58rem;
  background: transparent;
  color: var(--muted);
  font-size: 0.76rem;
  font-weight: 700;
}

.auth-mode-switch button.is-active {
  border-color: var(--line);
  background: #ffffff;
  color: var(--ink);
  box-shadow: 0 3px 12px rgba(28, 60, 55, 0.08);
}

.auth-close-button {
  display: grid;
  width: 2rem;
  height: 2rem;
  place-items: center;
  border: 0;
  border-radius: 50%;
  background: transparent;
  color: var(--muted);
  font-size: 1.45rem;
  line-height: 1;
}

.auth-close-button:hover {
  background: var(--bg);
  color: var(--ink);
}

.auth-card-body {
  padding: 1.3rem 1.8rem 1.65rem;
}

.auth-dialog-heading h2 {
  margin: 0;
  font-family: var(--font-body);
  font-size: clamp(1.7rem, 5vw, 2.15rem);
  font-weight: 780;
  letter-spacing: -0.04em;
}

.auth-dialog-heading > p:last-child {
  margin: 0.55rem 0 0;
  color: var(--muted);
  font-size: 0.82rem;
  line-height: 1.65;
}

.auth-notice {
  margin: 1rem 0 0;
  padding: 0.65rem 0.75rem;
  border-radius: var(--radius-sm);
  background: var(--accent-soft);
  color: var(--accent-strong);
  font-size: 0.76rem;
}

.auth-dialog-form {
  display: grid;
  gap: 0.9rem;
  margin-top: 1.15rem;
}

.auth-field {
  display: grid;
  gap: 0.4rem;
}

.auth-field > span {
  color: var(--muted-strong);
  font-size: 0.74rem;
  font-weight: 700;
}

.auth-field input {
  width: 100%;
  min-height: 2.8rem;
  padding: 0.65rem 2.8rem 0.65rem 0.78rem;
  border: 1px solid var(--line-strong);
  border-radius: var(--radius-sm);
  background: #f7faf9;
  color: var(--ink);
  outline: none;
}

.auth-field > input {
  padding-right: 0.78rem;
}

.auth-field input:focus {
  border-color: var(--accent);
  box-shadow: 0 0 0 3px rgba(22, 124, 108, 0.12);
}

.password-field {
  position: relative;
}

.password-toggle {
  position: absolute;
  top: 50%;
  right: 0.5rem;
  padding: 0.3rem 0.4rem;
  transform: translateY(-50%);
  border: 0;
  background: transparent;
  color: var(--accent-strong);
  font-size: 0.7rem;
  font-weight: 700;
}

.auth-submit-button {
  min-height: 2.9rem;
  margin-top: 0.15rem;
  border: 0;
  border-radius: var(--radius-sm);
  background: #16272b;
  color: #ffffff;
  font-weight: 750;
}

.auth-submit-button:hover {
  background: #263d42;
}

.auth-dialog-switch {
  margin: 1rem 0 0;
  color: var(--muted);
  font-size: 0.76rem;
  text-align: center;
}

.auth-dialog-switch button {
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--accent-strong);
  font-weight: 750;
}

@media (max-width: 520px) {
  .auth-dialog {
    width: calc(100vw - 1rem);
    max-height: calc(100vh - 1rem);
  }

  .auth-card-header {
    align-items: flex-start;
    padding: 1rem 1rem 0;
  }

  .auth-dialog-brand > span:last-child {
    display: none;
  }

  .auth-card-body {
    padding: 1.1rem 1rem 1.3rem;
  }
}
</style>
