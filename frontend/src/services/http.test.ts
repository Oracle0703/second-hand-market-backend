import { AxiosError, AxiosHeaders, type AxiosAdapter, type InternalAxiosRequestConfig } from 'axios'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuthStore } from '../stores/auth-store'
import { http } from './http'

function adapterResponse(config: InternalAxiosRequestConfig, data: unknown, status = 200) {
  return {
    data,
    status,
    statusText: status === 200 ? 'OK' : 'Unauthorized',
    headers: new AxiosHeaders(),
    config
  }
}

describe('http blob responses', () => {
  beforeEach(() => {
    localStorage.clear()
    useAuthStore.getState().clear()
  })

  afterEach(() => {
    vi.doUnmock('axios')
  })

  it('returns a successful blob without applying the JSON API envelope contract', async () => {
    const blob = new Blob(['license'], { type: 'image/jpeg' })
    const response = await http.get('/admin/files/7/content', {
      responseType: 'blob',
      adapter: async (config) => adapterResponse(config, blob)
    })

    expect(response.data).toBe(blob)
  })

  it('continues to reject a non-blob response with a nonzero business code', async () => {
    await expect(
      http.get('/admin/merchants', {
        adapter: async (config) =>
          adapterResponse(config, {
            code: 10003,
            message: 'forbidden',
            request_id: 'req-json',
            data: null
          })
      })
    ).rejects.toThrow('无权限访问')
  })

  it('refreshes concurrent blob 401 responses once and replays both with the new token', async () => {
    vi.resetModules()
    const actualAxios = await vi.importActual<typeof import('axios')>('axios')
    const mainClient = actualAxios.default.create()
    const refreshClient = actualAxios.default.create()
    const create = vi.fn().mockReturnValueOnce(mainClient).mockReturnValueOnce(refreshClient)
    vi.doMock('axios', () => ({ default: { create } }))

    const [{ http: isolatedHTTP }, { useAuthStore: isolatedAuthStore }] = await Promise.all([
      import('./http'),
      import('../stores/auth-store')
    ])
    isolatedAuthStore.getState().setAuth({
      accessToken: 'old-access',
      refreshToken: 'old-refresh',
      tokenScope: 'full',
      user: { id: 1, role: 'ADMIN' }
    })

    let mainAttempts = 0
    const replayAuthorizations: string[] = []
    const blob = new Blob(['private-license'], { type: 'image/jpeg' })
    mainClient.defaults.adapter = (async (config) => {
      mainAttempts += 1
      if (mainAttempts <= 2) {
        const response = adapterResponse(config, { code: 10002, message: 'unauthorized', request_id: 'req-401' }, 401)
        throw new AxiosError('unauthorized', AxiosError.ERR_BAD_REQUEST, config, undefined, response)
      }
      replayAuthorizations.push(String(config.headers.Authorization))
      return adapterResponse(config, blob)
    }) as AxiosAdapter

    let releaseRefresh!: () => void
    const refreshGate = new Promise<void>((resolve) => {
      releaseRefresh = resolve
    })
    let refreshCalls = 0
    const claims = btoa(JSON.stringify({ uid: 1, role: 'ADMIN', scope: 'full' }))
      .replace(/\+/g, '-')
      .replace(/\//g, '_')
      .replace(/=+$/, '')
    const refreshedAccess = `e30.${claims}.signature`
    refreshClient.defaults.adapter = (async (config) => {
      refreshCalls += 1
      await refreshGate
      return adapterResponse(config, {
        code: 0,
        message: 'OK',
        request_id: 'req-refresh',
        data: {
          access_token: refreshedAccess,
          refresh_token: 'new-refresh',
          expires_in: 7200
        }
      })
    }) as AxiosAdapter

    const requests = Promise.all([
      isolatedHTTP.get('/admin/files/7/content', { responseType: 'blob' }),
      isolatedHTTP.get('/admin/files/8/content', { responseType: 'blob' })
    ])
    await vi.waitFor(() => {
      expect(mainAttempts).toBe(2)
      expect(refreshCalls).toBe(1)
    })
    releaseRefresh()

    const responses = await requests
    expect(responses.map((response) => response.data)).toEqual([blob, blob])
    expect(refreshCalls).toBe(1)
    expect(replayAuthorizations).toEqual([`Bearer ${refreshedAccess}`, `Bearer ${refreshedAccess}`])
    expect(isolatedAuthStore.getState().refreshToken).toBe('new-refresh')
  })
})
