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

async function loadRequestModule(
  request: ReturnType<typeof vi.fn>,
  initialStorage: Record<string, unknown> = {}
) {
  const storage = new Map<string, unknown>(Object.entries(initialStorage))
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
  return { ...requestModule, ...sessionModule, storage, taro }
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
    { name: 'HTTP 401 without business auth code', statusCode: 401, code: 0 },
    { name: '2xx business unauthorized', statusCode: 200, code: 10002 }
  ])('refreshes and replays once for $name', async ({ statusCode, code }) => {
    const request = vi.fn(async (options: RequestOptions) => {
      if (options.url.endsWith('/buyer/auth/refresh')) {
        return apiResponse(200, 0, { access_token: 'new-access', refresh_token: 'new-refresh' })
      }
      if (options.header.Authorization === 'Bearer old-access') {
        return apiResponse(statusCode, code)
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

  test('replays a POST once without changing its method, path, or body', async () => {
    const request = vi.fn(async (options: RequestOptions) => {
      if (options.url.endsWith('/buyer/auth/refresh')) {
        return apiResponse(200, 0, { access_token: 'new-access', refresh_token: 'new-refresh' })
      }
      if (options.header.Authorization === 'Bearer old-access') {
        return apiResponse(401, 0)
      }
      return apiResponse(200, 0, { accepted: true })
    })
    const { apiRequest, useSessionStore } = await loadRequestModule(request)
    useSessionStore.getState().setSession('old-access', 'old-refresh', { id: 9, buyer_no: 'B0009' })
    const data = { product_id: 42, note: 'keep-me' }

    await expect(
      apiRequest<{ accepted: boolean }>({
        method: 'POST',
        path: '/buyer/intents',
        data
      })
    ).resolves.toEqual({ accepted: true })

    const protectedCalls = requestCalls(request).filter((call) => !call.url.endsWith('/buyer/auth/refresh'))
    expect(protectedCalls).toHaveLength(2)
    expect(protectedCalls.map(({ method, url, data: body }) => ({ method, url, body }))).toEqual([
      {
        method: 'POST',
        url: 'https://market.meaningful.ink/api/v1/buyer/intents',
        body: data
      },
      {
        method: 'POST',
        url: 'https://market.meaningful.ink/api/v1/buyer/intents',
        body: data
      }
    ])
  })

  test('hydrates stored credentials before the first authenticated request', async () => {
    const request = vi.fn(async (options: RequestOptions) => {
      if (options.url.endsWith('/buyer/auth/refresh')) {
        expect(options.data).toEqual({ refresh_token: 'stored-refresh' })
        return apiResponse(200, 0, { access_token: 'new-access', refresh_token: 'new-refresh' })
      }
      if (options.header.Authorization === 'Bearer stored-access') {
        return apiResponse(401, 0)
      }
      if (options.header.Authorization === 'Bearer new-access') {
        return apiResponse(200, 0, { hydrated: true })
      }
      throw new Error(`unexpected authorization: ${options.header.Authorization}`)
    })
    const { apiRequest } = await loadRequestModule(request, {
      buyer_access_token: 'stored-access',
      buyer_refresh_token: 'stored-refresh',
      buyer_profile: { id: 9, buyer_no: 'B0009' }
    })

    await expect(
      apiRequest<{ hydrated: boolean }>({ method: 'GET', path: '/buyer/favorites' })
    ).resolves.toEqual({ hydrated: true })

    expect(requestCalls(request)[0].header.Authorization).toBe('Bearer stored-access')
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

  test('reuses the completed flight when a concurrent old-session 401 arrives late', async () => {
    let releaseLate401!: () => void
    const late401Gate = new Promise<void>((resolve) => {
      releaseLate401 = resolve
    })
    let refreshCalls = 0
    const request = vi.fn(async (options: RequestOptions) => {
      if (options.url.endsWith('/buyer/auth/refresh')) {
        refreshCalls += 1
        return apiResponse(200, 0, { access_token: 'new-access', refresh_token: 'new-refresh' })
      }
      if (options.header.Authorization === 'Bearer old-access') {
        if (options.url.endsWith('/buyer/histories')) {
          await late401Gate
        }
        return apiResponse(401, 0)
      }
      if (options.header.Authorization === 'Bearer new-access') {
        return apiResponse(200, 0, { path: options.url })
      }
      throw new Error(`unexpected request: ${options.url}`)
    })
    const { apiRequest, useSessionStore } = await loadRequestModule(request)
    useSessionStore.getState().setSession('old-access', 'old-refresh', { id: 9, buyer_no: 'B0009' })

    const first = apiRequest<{ path: string }>({ method: 'GET', path: '/buyer/favorites' })
    const late = apiRequest<{ path: string }>({ method: 'GET', path: '/buyer/histories' })

    await expect(first).resolves.toEqual({
      path: 'https://market.meaningful.ink/api/v1/buyer/favorites'
    })
    expect(refreshCalls).toBe(1)
    releaseLate401()

    await expect(late).resolves.toEqual({
      path: 'https://market.meaningful.ink/api/v1/buyer/histories'
    })
    expect(refreshCalls).toBe(1)
    expect(requestCalls(request).filter((call) => call.header.Authorization === 'Bearer new-access')).toHaveLength(2)
  })

  test('serializes a replacement session refresh behind an active old-session flight', async () => {
    let releaseOldRefresh!: () => void
    const oldRefreshGate = new Promise<void>((resolve) => {
      releaseOldRefresh = resolve
    })
    let activeRefreshes = 0
    let maximumActiveRefreshes = 0
    const refreshTokens: string[] = []
    const request = vi.fn(async (options: RequestOptions) => {
      if (options.url.endsWith('/buyer/auth/refresh')) {
        const refreshToken = (options.data as { refresh_token: string }).refresh_token
        refreshTokens.push(refreshToken)
        activeRefreshes += 1
        maximumActiveRefreshes = Math.max(maximumActiveRefreshes, activeRefreshes)
        if (refreshToken === 'old-refresh') {
          await oldRefreshGate
        }
        activeRefreshes -= 1
        return apiResponse(200, 0, {
          access_token: `${refreshToken}-next-access`,
          refresh_token: `${refreshToken}-next`
        })
      }
      if (options.header.Authorization === 'Bearer replacement-refresh-next-access') {
        return apiResponse(200, 0, { owner: 'replacement' })
      }
      return apiResponse(401, 0)
    })
    const { apiRequest, AuthExpiredError, useSessionStore } = await loadRequestModule(request)
    useSessionStore.getState().setSession('old-access', 'old-refresh', { id: 9, buyer_no: 'B0009' })

    const oldRequest = apiRequest({ method: 'GET', path: '/buyer/favorites' })
    void oldRequest.catch(() => undefined)
    await vi.waitFor(() => expect(refreshTokens).toEqual(['old-refresh']))

    const replacementProfile = { id: 10, buyer_no: 'B0010' }
    useSessionStore.getState().setSession('replacement-access', 'replacement-refresh', replacementProfile)
    const replacementRequest = apiRequest<{ owner: string }>({ method: 'GET', path: '/buyer/histories' })
    await vi.waitFor(() => {
      expect(requestCalls(request).filter((call) => call.header.Authorization === 'Bearer replacement-access')).toHaveLength(1)
    })
    expect(refreshTokens).toEqual(['old-refresh'])

    releaseOldRefresh()

    await expect(oldRequest).rejects.toBeInstanceOf(AuthExpiredError)
    await expect(replacementRequest).resolves.toEqual({ owner: 'replacement' })
    expect(refreshTokens).toEqual(['old-refresh', 'replacement-refresh'])
    expect(maximumActiveRefreshes).toBe(1)
    expect(useSessionStore.getState()).toMatchObject({
      accessToken: 'replacement-refresh-next-access',
      refreshToken: 'replacement-refresh-next',
      profile: replacementProfile
    })
  })

  test.each([
    { name: 'transport rejection', kind: 'network' },
    { name: 'non-2xx success envelope', kind: 'http' },
    { name: 'malformed envelope', kind: 'malformed' },
    { name: 'nonzero business code', kind: 'business' },
    { name: 'empty access token', kind: 'empty-access' },
    { name: 'empty refresh token', kind: 'empty-refresh' }
  ])('clears the owned session when refresh has $name', async ({ kind }) => {
    const request = vi.fn(async (options: RequestOptions) => {
      if (options.url.endsWith('/buyer/auth/refresh')) {
        switch (kind) {
          case 'network':
            throw new Error('refresh network failed')
          case 'http':
            return apiResponse(401, 0, { access_token: 'new-access', refresh_token: 'new-refresh' })
          case 'malformed':
            return { statusCode: 200, header: {}, data: { unexpected: true } }
          case 'business':
            return apiResponse(200, 10002)
          case 'empty-access':
            return apiResponse(200, 0, { access_token: '', refresh_token: 'new-refresh' })
          case 'empty-refresh':
            return apiResponse(200, 0, { access_token: 'new-access', refresh_token: '' })
          default:
            throw new Error(`unknown refresh fixture: ${kind}`)
        }
      }
      if (options.header.Authorization === 'Bearer old-access') return apiResponse(401, 10002)
      return apiResponse(200, 0, { should_not_replay: true })
    })
    const { apiRequest, AuthExpiredError, useSessionStore, storage } = await loadRequestModule(request)
    useSessionStore.getState().setSession('old-access', 'old-refresh', {
      id: 9,
      buyer_no: 'B0009',
      nickname: 'Buyer'
    })

    const result = apiRequest({ method: 'GET', path: '/buyer/favorites' })

    await expect(result).rejects.toBeInstanceOf(AuthExpiredError)
    await expect(result).rejects.toMatchObject({
      name: 'AuthExpiredError',
      code: 10002,
      message: '登录已过期，请重新登录'
    })
    expect(useSessionStore.getState()).toMatchObject({
      accessToken: '',
      refreshToken: '',
      profile: undefined
    })
    expect(storage.has('buyer_access_token')).toBe(false)
    expect(storage.has('buyer_refresh_token')).toBe(false)
    expect(storage.has('buyer_profile')).toBe(false)
  })

  test('clears one failed refresh for all concurrent waiters', async () => {
    let releaseRefresh!: () => void
    const refreshGate = new Promise<void>((resolve) => {
      releaseRefresh = resolve
    })
    let refreshCalls = 0
    const request = vi.fn(async (options: RequestOptions) => {
      if (options.url.endsWith('/buyer/auth/refresh')) {
        refreshCalls += 1
        await refreshGate
        throw new Error('refresh unavailable')
      }
      return apiResponse(401, 10002)
    })
    const { apiRequest, AuthExpiredError, useSessionStore, taro } = await loadRequestModule(request)
    useSessionStore.getState().setSession('old-access', 'old-refresh', {
      id: 9,
      buyer_no: 'B0009',
      nickname: 'Buyer'
    })

    const first = apiRequest({ method: 'GET', path: '/buyer/favorites' })
    const second = apiRequest({ method: 'GET', path: '/buyer/histories' })
    void first.catch(() => undefined)
    void second.catch(() => undefined)
    await vi.waitFor(() => expect(refreshCalls).toBe(1))
    releaseRefresh()

    await expect(first).rejects.toBeInstanceOf(AuthExpiredError)
    await expect(second).rejects.toBeInstanceOf(AuthExpiredError)
    expect(refreshCalls).toBe(1)
    expect(taro.removeStorageSync.mock.calls).toEqual([
      ['buyer_access_token'],
      ['buyer_refresh_token'],
      ['buyer_profile']
    ])
  })

  test('clears stale credentials without calling refresh when no refresh token exists', async () => {
    const request = vi.fn(async () => apiResponse(401, 10002))
    const { apiRequest, AuthExpiredError, useSessionStore, storage } = await loadRequestModule(request)
    useSessionStore.getState().setSession('stale-access', '', {
      id: 9,
      buyer_no: 'B0009',
      nickname: 'Buyer'
    })

    await expect(apiRequest({ method: 'GET', path: '/buyer/favorites' })).rejects.toBeInstanceOf(AuthExpiredError)

    expect(requestCalls(request).filter((call) => call.url.endsWith('/buyer/auth/refresh'))).toHaveLength(0)
    expect(useSessionStore.getState().accessToken).toBe('')
    expect(storage.has('buyer_access_token')).toBe(false)
    expect(storage.has('buyer_profile')).toBe(false)
  })

  test.each([
    { name: 'HTTP 401 without business auth code', statusCode: 401, code: 0 },
    { name: '2xx business unauthorized', statusCode: 200, code: 10002 }
  ])('does not refresh recursively when the replay gets $name', async ({ statusCode, code }) => {
    let refreshCalls = 0
    let protectedCalls = 0
    const request = vi.fn(async (options: RequestOptions) => {
      if (options.url.endsWith('/buyer/auth/refresh')) {
        refreshCalls += 1
        return apiResponse(200, 0, { access_token: 'new-access', refresh_token: 'new-refresh' })
      }
      protectedCalls += 1
      return apiResponse(statusCode, code)
    })
    const { apiRequest, AuthExpiredError, useSessionStore } = await loadRequestModule(request)
    useSessionStore.getState().setSession('old-access', 'old-refresh', { id: 9, buyer_no: 'B0009' })

    await expect(apiRequest({ method: 'GET', path: '/buyer/favorites' })).rejects.toBeInstanceOf(AuthExpiredError)

    expect(refreshCalls).toBe(1)
    expect(protectedCalls).toBe(2)
    expect(useSessionStore.getState().accessToken).toBe('')
    expect(useSessionStore.getState().refreshToken).toBe('')
  })

  test.each([
    { name: 'HTTP 401', response: apiResponse(401, 0), message: 'request failed: status=401' },
    { name: '2xx business unauthorized', response: apiResponse(200, 10002), message: 'unauthorized' }
  ])('does not authenticate, refresh, or clear another session for skipAuth $name', async ({ response, message }) => {
    const request = vi.fn(async () => response)
    const { apiRequest, useSessionStore } = await loadRequestModule(request)
    const profile = { id: 9, buyer_no: 'B0009', nickname: 'Buyer' }
    useSessionStore.getState().setSession('active-access', 'active-refresh', profile)

    await expect(
      apiRequest({ method: 'POST', path: '/buyer/auth/miniapp-login', skipAuth: true })
    ).rejects.toThrow(message)

    expect(request).toHaveBeenCalledTimes(1)
    expect(requestCalls(request)[0].header.Authorization).toBeUndefined()
    expect(useSessionStore.getState()).toMatchObject({
      accessToken: 'active-access',
      refreshToken: 'active-refresh',
      profile
    })
  })

  test('does not restore a session cleared while refresh is pending', async () => {
    let releaseRefresh!: () => void
    const refreshGate = new Promise<void>((resolve) => {
      releaseRefresh = resolve
    })
    let refreshCalls = 0
    let protectedCalls = 0
    const request = vi.fn(async (options: RequestOptions) => {
      if (options.url.endsWith('/buyer/auth/refresh')) {
        refreshCalls += 1
        await refreshGate
        return apiResponse(200, 0, { access_token: 'late-access', refresh_token: 'late-refresh' })
      }
      protectedCalls += 1
      return apiResponse(401, 10002)
    })
    const { apiRequest, AuthExpiredError, useSessionStore } = await loadRequestModule(request)
    useSessionStore.getState().setSession('old-access', 'old-refresh', { id: 9, buyer_no: 'B0009' })

    const result = apiRequest({ method: 'GET', path: '/buyer/favorites' })
    void result.catch(() => undefined)
    await vi.waitFor(() => expect(refreshCalls).toBe(1))
    useSessionStore.getState().clearSession()
    releaseRefresh()

    await expect(result).rejects.toBeInstanceOf(AuthExpiredError)
    expect(protectedCalls).toBe(1)
    expect(useSessionStore.getState()).toMatchObject({ accessToken: '', refreshToken: '', profile: undefined })
  })

  test('does not clear a replacement session or replay the old request under its identity', async () => {
    let releaseRefresh!: () => void
    const refreshGate = new Promise<void>((resolve) => {
      releaseRefresh = resolve
    })
    let refreshCalls = 0
    const protectedAuthorizations: string[] = []
    const request = vi.fn(async (options: RequestOptions) => {
      if (options.url.endsWith('/buyer/auth/refresh')) {
        refreshCalls += 1
        await refreshGate
        return apiResponse(200, 0, { access_token: 'late-access', refresh_token: 'late-refresh' })
      }
      protectedAuthorizations.push(options.header.Authorization)
      return apiResponse(401, 10002)
    })
    const { apiRequest, AuthExpiredError, useSessionStore, storage } = await loadRequestModule(request)
    useSessionStore.getState().setSession('old-access', 'old-refresh', { id: 9, buyer_no: 'B0009' })

    const result = apiRequest({ method: 'GET', path: '/buyer/favorites' })
    void result.catch(() => undefined)
    await vi.waitFor(() => expect(refreshCalls).toBe(1))
    const replacementProfile = { id: 10, buyer_no: 'B0010', nickname: 'Other Buyer' }
    useSessionStore.getState().setSession('other-access', 'other-refresh', replacementProfile)
    releaseRefresh()

    await expect(result).rejects.toBeInstanceOf(AuthExpiredError)
    expect(protectedAuthorizations).toEqual(['Bearer old-access'])
    expect(useSessionStore.getState()).toMatchObject({
      accessToken: 'other-access',
      refreshToken: 'other-refresh',
      profile: replacementProfile
    })
    expect(storage.get('buyer_access_token')).toBe('other-access')
    expect(storage.get('buyer_refresh_token')).toBe('other-refresh')
    expect(storage.get('buyer_profile')).toEqual(replacementProfile)
  })

  test.each([
    { name: 'succeeds', refreshFails: false },
    { name: 'fails', refreshFails: true }
  ])('preserves a replacement session with the same refresh token when refresh $name', async ({ refreshFails }) => {
    let releaseRefresh!: () => void
    const refreshGate = new Promise<void>((resolve) => {
      releaseRefresh = resolve
    })
    const protectedAuthorizations: string[] = []
    const request = vi.fn(async (options: RequestOptions) => {
      if (options.url.endsWith('/buyer/auth/refresh')) {
        await refreshGate
        if (refreshFails) {
          throw new Error('refresh unavailable')
        }
        return apiResponse(200, 0, { access_token: 'late-access', refresh_token: 'late-refresh' })
      }
      protectedAuthorizations.push(options.header.Authorization)
      return apiResponse(401, 0)
    })
    const { apiRequest, AuthExpiredError, useSessionStore, storage } = await loadRequestModule(request)
    useSessionStore.getState().setSession('old-access', 'shared-refresh', { id: 9, buyer_no: 'B0009' })

    const result = apiRequest({ method: 'GET', path: '/buyer/favorites' })
    void result.catch(() => undefined)
    await vi.waitFor(() => {
      expect(requestCalls(request).filter((call) => call.url.endsWith('/buyer/auth/refresh'))).toHaveLength(1)
    })
    const replacementProfile = { id: 10, buyer_no: 'B0010', nickname: 'Other Buyer' }
    useSessionStore.getState().setSession('replacement-access', 'shared-refresh', replacementProfile)
    releaseRefresh()

    await expect(result).rejects.toBeInstanceOf(AuthExpiredError)
    expect(protectedAuthorizations).toEqual(['Bearer old-access'])
    expect(useSessionStore.getState()).toMatchObject({
      accessToken: 'replacement-access',
      refreshToken: 'shared-refresh',
      profile: replacementProfile
    })
    expect(storage.get('buyer_access_token')).toBe('replacement-access')
    expect(storage.get('buyer_refresh_token')).toBe('shared-refresh')
    expect(storage.get('buyer_profile')).toEqual(replacementProfile)
  })

  test('does not replay under a session synchronously replaced by a store subscriber', async () => {
    const protectedAuthorizations: string[] = []
    const request = vi.fn(async (options: RequestOptions) => {
      if (options.url.endsWith('/buyer/auth/refresh')) {
        return apiResponse(200, 0, { access_token: 'new-access', refresh_token: 'new-refresh' })
      }
      protectedAuthorizations.push(options.header.Authorization)
      return apiResponse(401, 0)
    })
    const { apiRequest, AuthExpiredError, useSessionStore } = await loadRequestModule(request)
    useSessionStore.getState().setSession('old-access', 'old-refresh', { id: 9, buyer_no: 'B0009' })
    const replacementProfile = { id: 10, buyer_no: 'B0010', nickname: 'Subscriber Buyer' }
    let replaced = false
    const unsubscribe = useSessionStore.subscribe((state) => {
      if (!replaced && state.accessToken === 'new-access') {
        replaced = true
        state.setSession('subscriber-access', 'subscriber-refresh', replacementProfile)
      }
    })

    const result = apiRequest({ method: 'GET', path: '/buyer/favorites' })

    await expect(result).rejects.toBeInstanceOf(AuthExpiredError)
    unsubscribe()
    expect(protectedAuthorizations).toEqual(['Bearer old-access'])
    expect(useSessionStore.getState()).toMatchObject({
      accessToken: 'subscriber-access',
      refreshToken: 'subscriber-refresh',
      profile: replacementProfile
    })
  })

  test.each([
    { name: 'HTTP 403', response: apiResponse(403, 10003), message: 'request failed: status=403' },
    { name: 'HTTP 500', response: apiResponse(500, 20001), message: 'request failed: status=500' },
    { name: 'malformed 2xx', response: { statusCode: 200, header: {}, data: { unexpected: true } }, message: 'API response malformed' }
  ])('preserves the existing $name error behavior', async ({ response: fixture, message }) => {
    const request = vi.fn(async () => fixture)
    const { apiRequest } = await loadRequestModule(request)

    await expect(apiRequest({ method: 'GET', path: '/buyer/products' })).rejects.toThrow(message)

    expect(request).toHaveBeenCalledTimes(1)
  })

  test('preserves the original Taro transport rejection', async () => {
    const transportError = new Error('transport unavailable')
    const request = vi.fn(async () => {
      throw transportError
    })
    const { apiRequest } = await loadRequestModule(request)

    await expect(apiRequest({ method: 'GET', path: '/buyer/products' })).rejects.toBe(transportError)
  })
})
