const accessTokenKey = 'shorturl_access_token'
const refreshTokenKey = 'shorturl_refresh_token'

// getAccessToken 读取本地 Access Token
export function getAccessToken(): string {
  return localStorage.getItem(accessTokenKey) ?? ''
}

// getRefreshToken 读取本地 Refresh Token
export function getRefreshToken(): string {
  return localStorage.getItem(refreshTokenKey) ?? ''
}

// saveTokens 保存轮换后的 Token 对
export function saveTokens(accessToken: string, refreshToken: string): void {
  localStorage.setItem(accessTokenKey, accessToken)
  localStorage.setItem(refreshTokenKey, refreshToken)
}

// clearTokens 清除本地登录凭证
export function clearTokens(): void {
  localStorage.removeItem(accessTokenKey)
  localStorage.removeItem(refreshTokenKey)
}
