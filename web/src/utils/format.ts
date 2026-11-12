// formatDateTime 将 UTC 时间转换为本地可读时间
export function formatDateTime(value: string | null): string {
  if (!value) {
    return '未设置'
  }
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value))
}

// formatCompactNumber 使用中文单位压缩较大的计数
export function formatCompactNumber(value: number): string {
  return new Intl.NumberFormat('zh-CN', {
    notation: value >= 10_000 ? 'compact' : 'standard',
    maximumFractionDigits: 1,
  }).format(value)
}

// statusLabel 返回链接状态文案
export function statusLabel(status: number): string {
  return status === 1 ? '启用' : '停用'
}

// shortDestination 截断较长目标地址用于列表展示
export function shortDestination(value: string, maxLength = 58): string {
  try {
    const parsed = new URL(value)
    const compact = `${parsed.host}${parsed.pathname === '/' ? '' : parsed.pathname}`
    return compact.length > maxLength
      ? `${compact.slice(0, maxLength - 1)}…`
      : compact
  } catch {
    return value.length > maxLength ? `${value.slice(0, maxLength - 1)}…` : value
  }
}
