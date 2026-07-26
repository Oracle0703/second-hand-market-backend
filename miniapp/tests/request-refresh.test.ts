import { afterEach, describe, expect, test, vi } from 'vitest'

type RequestOptions = {
  url: string
  method: string
  data?: unknown
  header: Record<string, string>
}

type TaroResponse = {
  statusCode: number
  data: unknown
  header: Record<string, string>
}

function apiResponse(statusCode: number, code: number, data: unknown = null): TaroResponse {
  return {
    statusCode,
    header: {},
    data: {
      code,
      message: code === 0 ? 'OK' : 'unauthorized',
      request_id: 'req-test',
      data
    }
  }
}

async function loadRequestModule(request: ReturnType<typeof vi.fn>) {
  const storage = new Map<string, unknown>()
  const taro = {
    request,
    getStorageSync: vi.fn((key: string) => storage.get(key)),
    setStorageSync: vi.fn((key: string, value: unknown) => storage.set(key, value)),
    removeStorageSync: vi.fn((key: string) => storage.delete(key))
  }
  vi.stubGlobal('__DEV_MODE__', false)
  vi.doMock('@tarojs/taro', () => ({ default: taro }))

  const requestModule = await import('../src/services/request')
  const sessionModule = await import('../src/stores/session')
  return { ...requestModule, ...sessionModule, storage }
}

function requestCalls(request: ReturnType<typeof vi.fn>): RequestOptions[] {
  return request.mock.calls.map(([options]) => options as RequestOptions)
}

afterEach(() => {
  vi.resetModules()
  vi.unmock('@tarojs/taro')
  vi.unstubAllGlobals()
})

describe('miniapp authenticated request refresh', () => {
  test.each([
    { name: 'HTTP 401', statusCode: 401 },
    { name: '2xx business unauthorized', statusCode: 200 }
  ])('refreshes and replays once for $name', async ({ statusCode }) => {
    const request = vi.fn(async (options: RequestOptions) => {
      if (options.url.endsWith('/buyer/auth/refresh')) {
        return apiResponse(200, 0, { access_token: 'new-access', refresh_token: 'new-refresh' })
      }
      if (options.header.Authorization === 'Bearer old-access') {
        return apiResponse(statusCode, 10002)
      }
      if (options.header.Authorization === 'Bearer new-access') {
        return apiResponse(200, 0, { item_id: 7 })
      }
      throw new Error(`unexpected request: ${options.url}`)
    })
    const { apiRequest, useSessionStore, storage } = await loadRequestModule(request)
    useSessionStore.getState().setSession('old-access', 'old-refresh', {
      id: 9,
      buyer_no: 'B0009',
      nickname: 'Buyer'
    })

    const result = await apiRequest<{ item_id: number }>({ method: 'GET', path: '/buyer/favorites' })

    expect(result).toEqual({ item_id: 7 })
    expect(request).toHaveBeenCalledTimes(3)
    const calls = requestCalls(request)
    const refresh = calls.find((call) => call.url.endsWith('/buyer/auth/refresh'))
    expect(refresh?.header.Authorization).toBeUndefined()
    expect(refresh?.header['X-Device-Id']).toMatch(/^dev_/)
    expect(calls[2].header.Authorization).toBe('Bearer new-access')
    expect(storage.get('buyer_access_token')).toBe('new-access')
    expect(storage.get('buyer_refresh_token')).toBe('new-refresh')
  })

  test('shares one refresh across concurrent HTTP 401 responses and replays both requests', async () => {
    let releaseRefresh!: () => void
    const refreshGate = new Promise<void>((resolve) => {
      releaseRefresh = resolve
    })
    let refreshCalls = 0
    let oldAccessCalls = 0
    const request = vi.fn(async (options: RequestOptions) => {
      if (options.url.endsWith('/buyer/auth/refresh')) {
        refreshCalls += 1
        await refreshGate
        return apiResponse(200, 0, { access_token: 'new-access', refresh_token: 'new-refresh' })
      }
      if (options.header.Authorization === 'Bearer old-access') {
        oldAccessCalls += 1
        return apiResponse(401, 10002)
      }
      if (options.header.Authorization === 'Bearer new-access') {
        return apiResponse(200, 0, { path: options.url })
      }
      throw new Error(`unexpected request: ${options.url}`)
    })
    const { apiRequest, useSessionStore } = await loadRequestModule(request)
    useSessionStore.getState().setSession('old-access', 'old-refresh', { id: 9, buyer_no: 'B0009' })

    const results = Promise.all([
      apiRequest<{ path: string }>({ method: 'GET', path: '/buyer/favorites' }),
      apiRequest<{ path: string }>({ method: 'GET', path: '/buyer/histories' })
    ])
    void results.catch(() => undefined)
    await vi.waitFor(() => {
      expect(oldAccessCalls).toBe(2)
      expect(refreshCalls).toBe(1)
    })
    releaseRefresh()

    await expect(results).resolves.toEqual([
      { path: 'https://market.meaningful.ink/api/v1/buyer/favorites' },
      { path: 'https://market.meaningful.ink/api/v1/buyer/histories' }
    ])
    expect(refreshCalls).toBe(1)
    expect(request).toHaveBeenCalledTimes(5)
    expect(requestCalls(request).slice(3).map((call) => call.header.Authorization)).toEqual([
      'Bearer new-access',
      'Bearer new-access'
    ])
  })
})
