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

http.interceptors.request.use((config) => {
  const token = useAuthStore.getState().accessToken
  if (token) {
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
    if (error.response?.status === 401) {
      useAuthStore.getState().clear()
    }
    return Promise.reject(error)
  }
)
