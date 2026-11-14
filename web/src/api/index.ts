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
  TrialClaimResponse,
  TrialLink,
  TrialLinksResponse,
  UpdateLinkInput,
  WorkspaceAnalyticsOverview,
  APIKeyListResponse,
  APIKeyScope,
  CreateAPIKeyResponse,
  CreateWebhookResponse,
  AbuseReport,
  AbuseReportListResponse,
  BlockedDomain,
  BlockedDomainListResponse,
  CreateAbuseReportResponse,
  WebhookDeliveryListResponse,
  WebhookEndpointListResponse,
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

// getWorkspaceAnalytics 获取当前工作空间整体分析
export function getWorkspaceAnalytics(
  days: number,
): Promise<WorkspaceAnalyticsOverview> {
  return requestJSON<WorkspaceAnalyticsOverview>(
    `/api/v1/analytics/overview?days=${days}`,
  )
}

// listAPIKeys 获取当前工作空间的 API Key 元数据
export function listAPIKeys(): Promise<APIKeyListResponse> {
  return requestJSON<APIKeyListResponse>('/api/v1/api-keys')
}

// createAPIKey 创建只显示一次 Secret 的 API Key
export function createAPIKey(
  name: string,
  scopes: APIKeyScope[],
): Promise<CreateAPIKeyResponse> {
  return requestJSON<CreateAPIKeyResponse>('/api/v1/api-keys', {
    method: 'POST',
    body: JSON.stringify({ name, scopes }),
  })
}

// revokeAPIKey 撤销指定 API Key
export function revokeAPIKey(id: string): Promise<void> {
  return requestJSON<void>(`/api/v1/api-keys/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

// listWebhooks 获取当前工作空间 Webhook 端点
export function listWebhooks(): Promise<WebhookEndpointListResponse> {
  return requestJSON<WebhookEndpointListResponse>('/api/v1/webhooks')
}

// createWebhook 创建端点并返回只显示一次的签名 Secret
export function createWebhook(
  name: string,
  url: string,
): Promise<CreateWebhookResponse> {
  return requestJSON<CreateWebhookResponse>('/api/v1/webhooks', {
    method: 'POST',
    body: JSON.stringify({ name, url }),
  })
}

// disableWebhook 停用端点并终止待投递任务
export function disableWebhook(id: string): Promise<void> {
  return requestJSON<void>(`/api/v1/webhooks/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

// listWebhookDeliveries 获取端点近期投递记录
export function listWebhookDeliveries(
  id: string,
  limit = 20,
): Promise<WebhookDeliveryListResponse> {
  return requestJSON<WebhookDeliveryListResponse>(
    `/api/v1/webhooks/${encodeURIComponent(id)}/deliveries?limit=${limit}`,
  )
}

// reportAbuse 提交公开的违规短链举报
export function reportAbuse(
  shortURL: string,
  reason: string,
  details: string,
): Promise<CreateAbuseReportResponse> {
  return requestJSON<CreateAbuseReportResponse>(
    '/api/v1/abuse/reports',
    {
      method: 'POST',
      body: JSON.stringify({
        short_url: shortURL,
        reason,
        details,
      }),
    },
    false,
  )
}

// listAbuseReports 获取管理员举报队列
export function listAbuseReports(
  status = 1,
  limit = 20,
): Promise<AbuseReportListResponse> {
  return requestJSON<AbuseReportListResponse>(
    `/api/v1/abuse/reports?status=${status}&limit=${limit}`,
  )
}

// resolveAbuseReport 处理举报并返回最终状态
export function resolveAbuseReport(
  id: string,
  decision: 'takedown' | 'reject',
  note: string,
): Promise<AbuseReport> {
  return requestJSON<AbuseReport>(
    `/api/v1/abuse/reports/${encodeURIComponent(id)}/resolve`,
    {
      method: 'POST',
      body: JSON.stringify({ decision, note }),
    },
  )
}

// listBlockedDomains 获取当前目标域名黑名单
export function listBlockedDomains(): Promise<BlockedDomainListResponse> {
  return requestJSON<BlockedDomainListResponse>(
    '/api/v1/abuse/blocked-domains',
  )
}

// addBlockedDomain 添加目标域名黑名单
export function addBlockedDomain(
  hostname: string,
  reason: string,
): Promise<BlockedDomain> {
  return requestJSON<BlockedDomain>('/api/v1/abuse/blocked-domains', {
    method: 'POST',
    body: JSON.stringify({ hostname, reason }),
  })
}

// removeBlockedDomain 停用目标域名黑名单
export function removeBlockedDomain(id: string): Promise<void> {
  return requestJSON<void>(
    `/api/v1/abuse/blocked-domains/${encodeURIComponent(id)}`,
    { method: 'DELETE' },
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

// claimTrialLinks 将当前游客 Cookie 下的链接迁移到登录工作空间
export function claimTrialLinks(): Promise<TrialClaimResponse> {
  return requestJSON<TrialClaimResponse>('/api/v1/trial/claim', { method: 'POST' })
}
