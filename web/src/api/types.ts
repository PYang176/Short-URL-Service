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

// ApiErrorPayload 是后端统一错误响应
export interface ApiErrorPayload {
  error?: {
    code?: string
    message?: string
    request_id?: string
  }
}
