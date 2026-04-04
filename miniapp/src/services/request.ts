import Taro from '@tarojs/taro'
import { ensureDeviceID, useSessionStore } from '../stores/session'

declare const __API_BASE_URL__: string
declare const __DEV_MODE__: boolean

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

type UnknownRecord = Record<string, unknown>

const BASE_URL = (typeof __API_BASE_URL__ === 'string' && __API_BASE_URL__.trim()) || 'https://market.meaningful.ink/api/v1'
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

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === 'object' && value !== null
}

function isAPIResponse<T>(payload: unknown): payload is APIResponse<T> {
  return isRecord(payload) && typeof payload.code === 'number'
}

function formatPayloadSummary(payload: unknown): string {
  if (typeof payload === 'string') {
    return `string:${payload.slice(0, 200)}`
  }
  if (isRecord(payload)) {
    return `object keys=${Object.keys(payload).slice(0, 10).join(',') || '(none)'}`
  }
  if (payload === null) {
    return 'null'
  }
  return typeof payload
}

function buildMalformedResponseError(res: Taro.request.SuccessCallbackResult<unknown>, url: string): Error {
  const detail = `API response malformed: status=${res.statusCode}, url=${url}, payload=${formatPayloadSummary(res.data)}`
  return new Error(detail)
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
    if (!isAPIResponse<{ access_token: string; refresh_token: string }>(payload)) {
      if (__DEV_MODE__) {
        console.error('[miniapp-api] refresh malformed response', res.statusCode, res.header, res.data)
      }
      clearSession()
      return false
    }
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
  const url = buildURL(path)
  if (__DEV_MODE__) {
    console.log('[miniapp-api] request', options.method, url)
  }
  let res
  try {
    res = await Taro.request<APIResponse<T>>({
      url,
      method: options.method,
      data: options.method === 'GET' ? undefined : options.data,
      header: headers
    })
  } catch (err) {
    if (__DEV_MODE__) {
      console.error('[miniapp-api] request failed', options.method, url, err)
    }
    throw err
  }
  const payload = res.data

  if (__DEV_MODE__) {
    console.log('[miniapp-api] response', options.method, url, res.statusCode, res.header, payload)
  }

  if (res.statusCode < 200 || res.statusCode >= 300) {
    throw new Error(`request failed: status=${res.statusCode}, url=${url}, payload=${formatPayloadSummary(payload)}`)
  }

  if (!isAPIResponse<T>(payload)) {
    throw buildMalformedResponseError(res as Taro.request.SuccessCallbackResult<unknown>, url)
  }

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
