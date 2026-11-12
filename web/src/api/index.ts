import { requestBlob, requestJSON } from './http'
import type {
  AnalyticsOverview,
  AuthResponse,
  CreateLinkInput,
  Domain,
  DomainListResponse,
  Link,
  LinkListResponse,
  ProfileResponse,
  TrialCreateInput,
  TrialLink,
  TrialLinksResponse,
  UpdateLinkInput,
} from './types'

// login 使用邮箱和密码登录
export function login(email: string, password: string): Promise<AuthResponse> {
  return requestJSON<AuthResponse>(
    '/api/v1/auth/login',
    { method: 'POST', body: JSON.stringify({ email, password }) },
    false,
  )
}

// register 创建账号和默认工作空间
export function register(
  email: string,
  password: string,
  displayName: string,
): Promise<AuthResponse> {
  return requestJSON<AuthResponse>(
    '/api/v1/auth/register',
    {
      method: 'POST',
      body: JSON.stringify({
        email,
        password,
        display_name: displayName,
      }),
    },
    false,
  )
}

// getProfile 获取当前用户与工作空间
export function getProfile(): Promise<ProfileResponse> {
  return requestJSON<ProfileResponse>('/api/v1/me')
}

// logout 撤销当前 Refresh Token
export function logout(refreshToken: string): Promise<void> {
  return requestJSON<void>(
    '/api/v1/auth/logout',
    {
      method: 'POST',
      body: JSON.stringify({ refresh_token: refreshToken }),
    },
    false,
  )
}

// listLinks 获取工作空间链接列表
export function listLinks(limit = 20, cursor = ''): Promise<LinkListResponse> {
  const query = new URLSearchParams({ limit: String(limit) })
  if (cursor) {
    query.set('cursor', cursor)
  }
  return requestJSON<LinkListResponse>(`/api/v1/links?${query}`)
}

// getLink 获取单个链接
export function getLink(id: string): Promise<Link> {
  return requestJSON<Link>(`/api/v1/links/${encodeURIComponent(id)}`)
}

// createLink 创建短链接
export function createLink(
  input: CreateLinkInput,
  idempotencyKey = '',
): Promise<Link> {
  const headers: Record<string, string> = {}
  if (idempotencyKey) {
    headers['Idempotency-Key'] = idempotencyKey
  }
  return requestJSON<Link>('/api/v1/links', {
    method: 'POST',
    headers,
    body: JSON.stringify(input),
  })
}

// updateLink 修改短链接
export function updateLink(id: string, input: UpdateLinkInput): Promise<Link> {
  return requestJSON<Link>(`/api/v1/links/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    body: JSON.stringify(input),
  })
}

// deleteLink 软删除短链接
export function deleteLink(id: string): Promise<void> {
  return requestJSON<void>(`/api/v1/links/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

// listDomains 获取工作空间自定义域名
export function listDomains(): Promise<DomainListResponse> {
  return requestJSON<DomainListResponse>('/api/v1/domains')
}

// addDomain 添加待验证域名
export function addDomain(hostname: string): Promise<Domain> {
  return requestJSON<Domain>('/api/v1/domains', {
    method: 'POST',
    body: JSON.stringify({ hostname }),
  })
}

// verifyDomain 验证域名 DNS 记录
export function verifyDomain(id: string): Promise<Domain> {
  return requestJSON<Domain>(
    `/api/v1/domains/${encodeURIComponent(id)}/verify`,
    { method: 'POST' },
  )
}

// deleteDomain 删除未使用域名
export function deleteDomain(id: string): Promise<void> {
  return requestJSON<void>(`/api/v1/domains/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

// getAnalytics 获取单个链接的分析结果
export function getAnalytics(id: string, days: number): Promise<AnalyticsOverview> {
  return requestJSON<AnalyticsOverview>(
    `/api/v1/links/${encodeURIComponent(id)}/analytics?days=${days}`,
  )
}

// getQRCode 获取链接二维码图片
export function getQRCode(id: string, format: 'png' | 'svg'): Promise<Blob> {
  return requestBlob(
    `/api/v1/links/${encodeURIComponent(id)}/qr?format=${format}&size=512`,
  )
}

// createTrialLink 在无需登录的游客会话中创建短期短链
export function createTrialLink(input: TrialCreateInput): Promise<TrialLink> {
  return requestJSON<TrialLink>(
    '/api/v1/trial/links',
    {
      method: 'POST',
      body: JSON.stringify(input),
    },
    false,
  )
}

// listTrialLinks 获取当前浏览器游客会话的短期短链
export function listTrialLinks(): Promise<TrialLinksResponse> {
  return requestJSON<TrialLinksResponse>('/api/v1/trial/links', {}, false)
}
