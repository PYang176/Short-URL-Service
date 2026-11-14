// User 是当前登录用户
export interface User {
  id: string
  email: string
  display_name: string
  role: number
  created_at: string
}

// Workspace 是链接与统计数据的归属空间
export interface Workspace {
  id: string
  name: string
  slug: string
  created_at: string
}

// TokenPair 是登录接口返回的凭证
export interface TokenPair {
  access_token: string
  token_type: string
  expires_at: string
  refresh_token: string
}

// AuthResponse 是注册、登录和刷新接口的响应
export interface AuthResponse {
  user: User
  workspace: Workspace
  token: TokenPair
}

// ProfileResponse 是当前用户和默认工作空间
export interface ProfileResponse {
  user: User
  workspace: Workspace
}

// Link 是短链接管理数据
export interface Link {
  id: string
  slug: string
  short_url: string
  domain_id: string
  domain_hostname: string
  destination_url: string
  title: string
  notes: string
  status: number
  redirect_code: number
  starts_at: string | null
  expires_at: string | null
  created_at: string
}

// Domain 是工作空间自定义域名
export interface Domain {
  id: string
  hostname: string
  status: number
  verified: boolean
  txt_name: string
  txt_value: string
  cname: string
  last_verified_at: string | null
  created_at: string
}

// DomainListResponse 是域名接口响应
export interface DomainListResponse {
  items: Domain[]
}

// LinkListResponse 是游标分页的链接列表
export interface LinkListResponse {
  items: Link[]
  next_cursor: string
}

// CreateLinkInput 是创建短链表单提交的数据
export interface CreateLinkInput {
  destination_url: string
  domain_id?: string
  alias?: string
  title?: string
  notes?: string
  redirect_code: number
  starts_at?: string | null
  expires_at?: string | null
}

// UpdateLinkInput 是修改短链时允许提交的字段
export interface UpdateLinkInput {
  destination_url?: string
  title?: string
  notes?: string
  status?: number
  redirect_code?: number
  expires_at?: string | null
  clear_expires_at?: boolean
}

// TrialCreateInput 是匿名游客创建短期短链的输入
export interface TrialCreateInput {
  destination_url: string
  alias?: string
  title?: string
  expires_in_minutes: 15 | 30 | 60 | 720 | 1440
}

// TrialLink 是游客会话创建的短期短链
export interface TrialLink {
  id: string
  slug: string
  short_url: string
  destination_url: string
  title: string
  expires_at: string
  created_at: string
}

// TrialLinksResponse 是游客当前会话的链接和剩余额度
export interface TrialLinksResponse {
  items: TrialLink[]
  remaining: number
  limit: number
  expires_at: string | null
}

// TrialClaimResponse 是游客链接迁移到正式工作空间的结果
export interface TrialClaimResponse {
  claimed_links: number
  claimed_clicks: number
}

// SummaryMetrics 是链接分析核心指标
export interface SummaryMetrics {
  total_clicks: number
  human_clicks: number
  bot_clicks: number
  unique_visitors: number
}

// TrendPoint 是趋势图上的一个时间桶
export interface TrendPoint {
  bucket: string
  human_clicks: number
  bot_clicks: number
}

// BreakdownItem 是维度排行中的一项
export interface BreakdownItem {
  value: string
  clicks: number
}

// AnalyticsOverview 是链接分析接口响应
export interface AnalyticsOverview {
  days: number
  summary: SummaryMetrics
  trend: TrendPoint[]
  countries: BreakdownItem[]
  devices: BreakdownItem[]
  operating_systems: BreakdownItem[]
  browsers: BreakdownItem[]
  referrers: BreakdownItem[]
}

// WorkspaceAnalyticsSummary 是工作空间整体指标
export interface WorkspaceAnalyticsSummary {
  total_links: number
  active_links: number
  total_clicks: number
  human_clicks: number
  bot_clicks: number
  unique_visitors: number
}

// WorkspaceTopLink 是工作空间点击排行中的链接
export interface WorkspaceTopLink {
  id: string
  slug: string
  title: string
  status: number
  clicks: number
}

// WorkspaceAnalyticsOverview 是工作空间整体分析响应
export interface WorkspaceAnalyticsOverview {
  days: number
  summary: WorkspaceAnalyticsSummary
  trend: TrendPoint[]
  top_links: WorkspaceTopLink[]
}

// AnalyticsExportFilter 是点击明细导出的组合筛选条件
export interface AnalyticsExportFilter {
  link_id?: string
  days: number
  visitor: 'all' | 'human' | 'bot'
  device: '' | 'desktop' | 'mobile' | 'tablet'
  country: string
  browser: string
  utm_source: string
}

// APIKeyScope 是当前后端支持的 API Key 权限
export type APIKeyScope =
  | 'links:read'
  | 'links:write'
  | 'analytics:read'

// APIKeyRecord 是 API Key 的安全元数据
export interface APIKeyRecord {
  id: string
  name: string
  prefix: string
  scopes: APIKeyScope[]
  last_used_at: string | null
  expires_at: string | null
  revoked_at: string | null
  created_at: string
}

// APIKeyListResponse 是 API Key 列表响应
export interface APIKeyListResponse {
  items: APIKeyRecord[]
}

// CreateAPIKeyResponse 是只返回一次明文 Secret 的创建响应
export interface CreateAPIKeyResponse {
  key: APIKeyRecord
  secret: string
}

// WebhookEndpoint 是工作空间回调端点
export interface WebhookEndpoint {
  id: string
  name: string
  url: string
  status: number
  failure_count: number
  disabled_at: string | null
  created_at: string
}

// WebhookEndpointListResponse 是端点列表响应
export interface WebhookEndpointListResponse {
  items: WebhookEndpoint[]
}

// CreateWebhookResponse 是只返回一次签名 Secret 的创建响应
export interface CreateWebhookResponse {
  endpoint: WebhookEndpoint
  secret: string
}

// WebhookDelivery 是一次 Outbox 投递记录
export interface WebhookDelivery {
  id: number
  event_id: string
  event_type: string
  status: number
  attempt_count: number
  next_attempt_at: string
  response_status: number | null
  last_error: string
  delivered_at: string | null
  created_at: string
}

// WebhookDeliveryListResponse 是端点近期投递记录
export interface WebhookDeliveryListResponse {
  items: WebhookDelivery[]
}

// AbuseReport 是管理员处理的违规短链举报
export interface AbuseReport {
  id: string
  link_id: string
  link_title: string
  link_slug: string
  domain_hostname: string
  destination_url: string
  link_status: number
  reason: string
  details: string
  status: number
  resolution_note: string
  resolved_at: string | null
  created_at: string
}

// AbuseReportListResponse 是举报队列响应
export interface AbuseReportListResponse {
  items: AbuseReport[]
}

// CreateAbuseReportResponse 是公开举报提交结果
export interface CreateAbuseReportResponse {
  id: string
  status: number
}

// BlockedDomain 是禁止作为短链目标的域名规则
export interface BlockedDomain {
  id: string
  hostname: string
  reason: string
  created_at: string
}

// BlockedDomainListResponse 是黑名单列表响应
export interface BlockedDomainListResponse {
  items: BlockedDomain[]
}

// ApiErrorPayload 是后端统一错误响应
export interface ApiErrorPayload {
  error?: {
    code?: string
    message?: string
    request_id?: string
  }
}
