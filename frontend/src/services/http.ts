import axios from 'axios'
import { useAuthStore } from '../stores/auth-store'
import { ERROR_MESSAGES } from '../constants/error-codes'
import type { AuthUser } from '../types/auth'

const DEFAULT_HTTP_TIMEOUT_MS = 15000
const DEFAULT_UPLOAD_TIMEOUT_MS = 300000
const DEFAULT_REFRESH_TIMEOUT_MS = 60000
const parsedUploadTimeout = Number(import.meta.env.VITE_UPLOAD_TIMEOUT_MS)
const parsedRefreshTimeout = Number(import.meta.env.VITE_REFRESH_TIMEOUT_MS)
const UPLOAD_TIMEOUT_MS =
  Number.isFinite(parsedUploadTimeout) && parsedUploadTimeout > 0 ? parsedUploadTimeout : DEFAULT_UPLOAD_TIMEOUT_MS
const REFRESH_TIMEOUT_MS =
  Number.isFinite(parsedRefreshTimeout) && parsedRefreshTimeout > 0 ? parsedRefreshTimeout : DEFAULT_REFRESH_TIMEOUT_MS

export type APIResponse<T> = {
  code: number
  message: string
  request_id: string
  data: T
}

export const http = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080/api/v1',
  timeout: DEFAULT_HTTP_TIMEOUT_MS
})
const refreshClient = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080/api/v1',
  timeout: REFRESH_TIMEOUT_MS
})

const AUTH_EXEMPT_PATHS = new Set([
  '/auth/login',
  '/auth/register',
  '/auth/refresh',
  '/buyer/auth/wechat-login',
  '/buyer/auth/refresh'
])

function getPathname(url?: string) {
  if (!url) return ''
  try {
    return new URL(url, 'http://localhost').pathname
  } catch {
    return url
  }
}

function isAuthExempt(url?: string) {
  return AUTH_EXEMPT_PATHS.has(getPathname(url))
}

type AccessTokenClaims = {
  uid?: number
  ut?: string
  role?: string
  mid?: number
  scope?: string
}

function decodeAccessTokenClaims(token: string): AccessTokenClaims | null {
  const parts = token.split('.')
  if (parts.length < 2) return null
  try {
    const normalized = parts[1].replace(/-/g, '+').replace(/_/g, '/')
    const padded = normalized.padEnd(Math.ceil(normalized.length / 4) * 4, '=')
    const json = atob(padded)
    return JSON.parse(json) as AccessTokenClaims
  } catch {
    return null
  }
}

function buildUserFromClaims(claims: AccessTokenClaims | null, fallback: AuthUser | null): AuthUser | null {
  if (!claims || typeof claims.uid !== 'number' || !claims.role) return fallback
  const nextUser: AuthUser = {
    id: claims.uid,
    role: String(claims.role)
  }
  if (typeof claims.mid === 'number' && claims.mid > 0) {
    nextUser.merchant_id = claims.mid
  }
  return nextUser
}

let refreshPromise: Promise<string | null> | null = null

async function refreshAccessToken(): Promise<string | null> {
  const { refreshToken, user, tokenScope } = useAuthStore.getState()
  if (!refreshToken) return null
  try {
    const res = await refreshClient.post<APIResponse<{ access_token: string; refresh_token: string; expires_in: number }>>('/auth/refresh', {
      refresh_token: refreshToken
    })
    if (res.data.code !== 0) {
      throw new Error(ERROR_MESSAGES[res.data.code] ?? res.data.message)
    }
    const accessToken = res.data.data.access_token
    const nextRefreshToken = res.data.data.refresh_token
    const claims = decodeAccessTokenClaims(accessToken)
    const nextUser = buildUserFromClaims(claims, user)
    const nextScope = claims?.scope === 'full' || claims?.scope === 'onboarding' ? claims.scope : (tokenScope || 'full')
    if (!nextUser) {
      useAuthStore.getState().clear()
      return null
    }
    useAuthStore.getState().setAuth({
      accessToken,
      refreshToken: nextRefreshToken,
      tokenScope: nextScope,
      user: nextUser
    })
    return accessToken
  } catch {
    useAuthStore.getState().clear()
    return null
  }
}

http.interceptors.request.use((config) => {
  const token = useAuthStore.getState().accessToken
  if (token && !isAuthExempt(config.url)) {
    config.headers.Authorization = `Bearer ${token}`
  }
  if (getPathname(config.url) === '/files/upload') {
    config.timeout = UPLOAD_TIMEOUT_MS
  }
  return config
})

http.interceptors.response.use(
  (response) => {
    if (response.config.responseType === 'blob' && response.data instanceof Blob) {
      return response
    }
    const payload = response.data as APIResponse<unknown>
    if (payload.code !== 0) {
      const msg = ERROR_MESSAGES[payload.code] ?? payload.message
      return Promise.reject(new Error(msg))
    }
    return response
  },
  async (error) => {
    const payload = error.response?.data as Partial<APIResponse<unknown>> | undefined
    const requestPath = getPathname(error.config?.url)
    const status = error.response?.status
    const originalConfig = error.config as (typeof error.config & { _retry?: boolean }) | undefined
    const isTimeout = error.code === 'ECONNABORTED' || (typeof error.message === 'string' && error.message.toLowerCase().includes('timeout'))

    if (status === 401 && originalConfig && !originalConfig._retry && !isAuthExempt(originalConfig.url) && requestPath !== '/auth/refresh') {
      originalConfig._retry = true
      if (!refreshPromise) {
        refreshPromise = refreshAccessToken().finally(() => {
          refreshPromise = null
        })
      }
      const newAccessToken = await refreshPromise
      if (newAccessToken) {
        originalConfig.headers = originalConfig.headers ?? {}
        originalConfig.headers.Authorization = `Bearer ${newAccessToken}`
        return http(originalConfig)
      }
      return Promise.reject(new Error('登录已过期，请重新登录'))
    }

    if (isTimeout) {
      const msg = requestPath === '/files/upload' ? '上传超时，请检查网络后重试' : '请求超时，请稍后重试'
      return Promise.reject(new Error(msg))
    }

    if (payload && typeof payload.code === 'number') {
      const isLoginRequest = requestPath === '/auth/login'
      const msg = isLoginRequest && payload.code === 10002 ? '账号或密码错误' : (ERROR_MESSAGES[payload.code] ?? payload.message ?? error.message)
      if (status === 401) {
        useAuthStore.getState().clear()
      }
      return Promise.reject(new Error(msg))
    }
    if (status === 401) {
      useAuthStore.getState().clear()
    }
    return Promise.reject(error)
  }
)
