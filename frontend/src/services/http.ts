import axios from 'axios'
import { useAuthStore } from '../stores/auth-store'
import { ERROR_MESSAGES } from '../constants/error-codes'

export type APIResponse<T> = {
  code: number
  message: string
  request_id: string
  data: T
}

export const http = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080/api/v1',
  timeout: 15000
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

http.interceptors.request.use((config) => {
  const token = useAuthStore.getState().accessToken
  if (token && !isAuthExempt(config.url)) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

http.interceptors.response.use(
  (response) => {
    const payload = response.data as APIResponse<unknown>
    if (payload.code !== 0) {
      const msg = ERROR_MESSAGES[payload.code] ?? payload.message
      return Promise.reject(new Error(msg))
    }
    return response
  },
  (error) => {
    const payload = error.response?.data as Partial<APIResponse<unknown>> | undefined
    if (payload && typeof payload.code === 'number') {
      const requestPath = getPathname(error.config?.url)
      const isLoginRequest = requestPath === '/auth/login'
      const msg = isLoginRequest && payload.code === 10002 ? '账号或密码错误' : (ERROR_MESSAGES[payload.code] ?? payload.message ?? error.message)
      if (error.response?.status === 401) {
        useAuthStore.getState().clear()
      }
      return Promise.reject(new Error(msg))
    }
    if (error.response?.status === 401) {
      useAuthStore.getState().clear()
    }
    return Promise.reject(error)
  }
)
