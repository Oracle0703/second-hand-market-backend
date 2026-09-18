import { waitFor } from '@testing-library/react'
import { AxiosError, type AxiosResponse, type InternalAxiosRequestConfig } from 'axios'
import { afterEach, expect, it } from 'vitest'
import { http } from './http'
import { useAuthStore } from '../stores/auth-store'

function login(id: number) {
  useAuthStore.getState().setAuth({ accessToken: `token-${id}`, refreshToken: `refresh-${id}`, user: { id, role: 'MERCHANT', merchant_id: id } })
}
afterEach(() => useAuthStore.getState().clear())

it('rejects a late successful response from a previous account', async () => {
  login(1)
  let finish!: (response: AxiosResponse) => void
  let sent!: InternalAxiosRequestConfig
  const pending = http.get('/merchant/products', { adapter: (config) => {
    sent = config
    return new Promise((resolve) => { finish = resolve })
  } })
  await waitFor(() => expect(sent).toBeDefined())
  login(2)
  finish({ config: sent, data: { code: 0, data: ['private-old-data'] }, status: 200, statusText: 'OK', headers: {} })
  await expect(pending).rejects.toThrow('会话已切换')
  expect(useAuthStore.getState().user?.id).toBe(2)
})

it('does not refresh or clear a new account for an old unauthorized request', async () => {
  login(1)
  let fail!: (error: Error) => void
  let sent!: InternalAxiosRequestConfig
  const pending = http.get('/merchant/products', { adapter: (config) => {
    sent = config
    return new Promise((_, reject) => { fail = reject })
  } })
  await waitFor(() => expect(sent).toBeDefined())
  login(2)
  fail(new AxiosError('expired', 'ERR_BAD_RESPONSE', sent, undefined, { config: sent, data: { code: 10002 }, status: 401, statusText: 'Unauthorized', headers: {} }))
  await expect(pending).rejects.toThrow('会话已切换')
  expect(useAuthStore.getState().user?.id).toBe(2)
})
