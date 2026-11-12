import type { ApiErrorPayload, AuthResponse } from './types'
import {
  clearTokens,
  getAccessToken,
  getRefreshToken,
  saveTokens,
} from './token-storage'

const apiBaseURL = (import.meta.env.VITE_API_BASE_URL ?? '').replace(/\/$/, '')
let refreshPromise: Promise<boolean> | null = null

// ApiError 保留 HTTP 状态和请求 ID，便于界面显示稳定错误
export class ApiError extends Error {
  readonly status: number
  readonly code: string
  readonly requestID: string

  constructor(status: number, code: string, message: string, requestID = '') {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.requestID = requestID
  }
}

// requestJSON 执行需要统一错误处理的 JSON API 请求
export async function requestJSON<T>(
  path: string,
  init: RequestInit = {},
  authenticated = true,
): Promise<T> {
  const response = await executeRequest(path, init, authenticated)
  if (response.status === 401 && authenticated && (await refreshSession())) {
    return parseResponse<T>(await executeRequest(path, init, authenticated))
  }
  return parseResponse<T>(response)
}

// requestBlob 下载二进制响应，并在 401 后刷新一次 Token
export async function requestBlob(path: string): Promise<Blob> {
  let response = await executeRequest(path, {}, true)
  if (response.status === 401 && (await refreshSession())) {
    response = await executeRequest(path, {}, true)
  }
  if (!response.ok) {
    throw await toApiError(response)
  }
  return response.blob()
}

// executeRequest 添加认证头和 JSON 请求头
async function executeRequest(
  path: string,
  init: RequestInit,
  authenticated: boolean,
): Promise<Response> {
  const headers = new Headers(init.headers)
  if (init.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  if (authenticated) {
    const token = getAccessToken()
    if (token) {
      headers.set('Authorization', `Bearer ${token}`)
    }
  }
  return fetch(`${apiBaseURL}${path}`, {
    ...init,
    headers,
  })
}

// parseResponse 将成功响应解析为 JSON 或空值
async function parseResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    throw await toApiError(response)
  }
  if (response.status === 204) {
    return undefined as T
  }
  return (await response.json()) as T
}

// toApiError 将后端错误体转换为前端错误
async function toApiError(response: Response): Promise<ApiError> {
  let payload: ApiErrorPayload = {}
  try {
    payload = (await response.json()) as ApiErrorPayload
  } catch {
    // 非 JSON 错误体使用通用文案。
  }
  const error = payload.error
  return new ApiError(
    response.status,
    error?.code ?? 'REQUEST_FAILED',
    error?.message ?? '请求失败，请稍后重试',
    error?.request_id ?? '',
  )
}

// refreshSession 使用 Refresh Token 轮换登录凭证
async function refreshSession(): Promise<boolean> {
  if (refreshPromise) {
    return refreshPromise
  }
  const refreshToken = getRefreshToken()
  if (!refreshToken) {
    return false
  }

  refreshPromise = (async () => {
    try {
      const response = await fetch(`${apiBaseURL}/api/v1/auth/refresh`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ refresh_token: refreshToken }),
      })
      if (!response.ok) {
        expireSession()
        return false
      }
      const result = (await response.json()) as AuthResponse
      saveTokens(result.token.access_token, result.token.refresh_token)
      return true
    } catch {
      expireSession()
      return false
    } finally {
      refreshPromise = null
    }
  })()
  return refreshPromise
}

// expireSession 清除失效凭证并通知应用跳回登录页
function expireSession(): void {
  clearTokens()
  window.dispatchEvent(new Event('shorturl:auth-expired'))
}
