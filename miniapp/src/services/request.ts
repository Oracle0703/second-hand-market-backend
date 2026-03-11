import Taro from '@tarojs/taro'
import { ensureDeviceID, useSessionStore } from '../stores/session'

type APIResponse<T> = {
  code: number
  message: string
  request_id: string
  data: T
}

type RequestOptions<T> = {
  method: 'GET' | 'POST' | 'DELETE'
  path: string
  data?: Record<string, unknown>
  skipAuth?: boolean
  retrying?: boolean
}

const BASE_URL = process.env.TARO_APP_API_BASE_URL || 'http://localhost:8080/api/v1'
let refreshingPromise: Promise<boolean> | null = null

function buildURL(path: string): string {
  if (path.startsWith('http')) return path
  return `${BASE_URL}${path}`
}

function withQuery(path: string, data?: Record<string, unknown>): string {
  if (!data || Object.keys(data).length === 0) return path
  const params = Object.entries(data)
    .filter(([, value]) => value !== undefined && value !== null && value !== '')
    .map(([key, value]) => `${encodeURIComponent(key)}=${encodeURIComponent(String(value))}`)
    .join('&')
  if (!params) return path
  return `${path}${path.includes('?') ? '&' : '?'}${params}`
}

async function refreshAccessToken(): Promise<boolean> {
  const { refreshToken, clearSession, setSession, profile } = useSessionStore.getState()
  if (!refreshToken) return false
  try {
    const res = await Taro.request<APIResponse<{ access_token: string; refresh_token: string }>>({
      url: buildURL('/buyer/auth/refresh'),
      method: 'POST',
      data: { refresh_token: refreshToken },
      header: {
        'Content-Type': 'application/json',
        'X-Device-Id': ensureDeviceID()
      }
    })
    const payload = res.data
    if (payload.code !== 0) {
      clearSession()
      return false
    }
    setSession(payload.data.access_token, payload.data.refresh_token, profile)
    return true
  } catch {
    clearSession()
    return false
  }
}

export async function apiRequest<T>(options: RequestOptions<T>): Promise<T> {
  const state = useSessionStore.getState()
  const deviceID = ensureDeviceID()
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    'X-Device-Id': deviceID
  }
  if (!options.skipAuth && state.accessToken) {
    headers.Authorization = `Bearer ${state.accessToken}`
  }

  const path = options.method === 'GET' ? withQuery(options.path, options.data) : options.path
  const res = await Taro.request<APIResponse<T>>({
    url: buildURL(path),
    method: options.method,
    data: options.method === 'GET' ? undefined : options.data,
    header: headers
  })
  const payload = res.data

  if (payload.code === 0) {
    return payload.data
  }

  if (payload.code === 10002 && !options.skipAuth && !options.retrying && state.refreshToken) {
    if (!refreshingPromise) {
      refreshingPromise = refreshAccessToken().finally(() => {
        refreshingPromise = null
      })
    }
    const ok = await refreshingPromise
    if (ok) {
      return apiRequest<T>({ ...options, retrying: true })
    }
  }

  throw new Error(payload.message || `request failed: ${payload.code}`)
}
