import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const storage = new Map<string, unknown>()
const request = vi.fn()
let env: 'weapp' | 'tt'
let onShow: () => void

vi.mock('@tarojs/taro', () => ({
  ENV_TYPE: { WEAPP: 'weapp', TT: 'tt' },
  useDidShow: (callback: () => void) => { onShow = callback },
  default: {
    getEnv: () => env,
    getStorageSync: (key: string) => storage.get(key),
    setStorageSync: (key: string, value: unknown) => storage.set(key, structuredClone(value))
  }
}))
vi.mock('../src/services/request', () => ({ apiRequest: (...args: unknown[]) => request(...args) }))

const home = 'pages/home/index'
const detail = 'pages/product/detail/index'
const load = () => import('../src/services/analytics')

describe('小程序埋点队列', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.resetModules()
    vi.stubGlobal('__MERCHANT_NO__', 'M_test')
    env = 'weapp'
    storage.clear()
    request.mockReset().mockResolvedValue({})
  })

  afterEach(() => {
    vi.clearAllTimers()
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it.each(['weapp', 'tt'] as const)('%s 页面每次可见各记一次，点击与曝光合并匿名上报', async (platform) => {
    env = platform
    const sdk = await load()
    const { usePageExposure } = await import('../src/hooks/usePageExposure')
    usePageExposure(detail, 12)
    expect(request).not.toHaveBeenCalled()
    onShow()
    onShow()
    sdk.trackClick('contact_click', detail, 'detail_want', 12)
    await vi.advanceTimersByTimeAsync(2000)
    expect(request).toHaveBeenCalledTimes(1)
    const [payload] = request.mock.calls[0]
    expect(payload).toMatchObject({
      method: 'POST', path: '/buyer/analytics/events?merchant_no=M_test', skipAuth: true,
      data: { platform, events: [
        { name: 'page_view', page: detail, product_id: 12 },
        { name: 'page_view', page: detail, product_id: 12 },
        { name: 'contact_click', page: detail, product_id: 12, source: 'detail_want' }
      ] }
    })
    expect(new Set(payload.data.events.map((event: { event_id: string }) => event.event_id)).size).toBe(3)
    expect([...storage.values()]).toEqual([[]])
  })

  it('请求期间新点击保留，重复 flush 共用一次请求', async () => {
    const sdk = await load()
    let acknowledge!: () => void
    request.mockImplementationOnce(() => new Promise<void>((resolve) => { acknowledge = resolve }))
    sdk.trackPageView(home)
    const first = sdk.flushAnalytics()
    sdk.trackClick('contact_click', home, 'home_store_phone')
    expect(sdk.flushAnalytics()).toBe(first)
    acknowledge()
    await first
    await sdk.flushAnalytics()
    expect(request.mock.calls.map(([payload]) => payload.data.events.length)).toEqual([1, 1])
    expect(request.mock.calls[1][0].data.events[0].name).toBe('contact_click')
  })

  it('断网后保留 ID，重新启动在原商家和平台恢复；切换商家不发送旧数据', async () => {
    const sdk = await load()
    request.mockRejectedValueOnce(new Error('offline'))
    sdk.trackPageView(home)
    sdk.pauseAnalytics()
    await sdk.flushAnalytics()
    const first = request.mock.calls[0][0].data.events
    vi.resetModules()
    vi.stubGlobal('__MERCHANT_NO__', 'another')
    await (await load()).flushAnalytics()
    expect(request).toHaveBeenCalledTimes(1)
    vi.stubGlobal('__MERCHANT_NO__', 'M_test')
    vi.resetModules()
    const restarted = await load()
    restarted.resumeAnalytics()
    await vi.advanceTimersByTimeAsync(2000)
    expect(request.mock.calls[1][0].data.events).toEqual(first)
  })

  it('滞留队列按 50 条拆批并淘汰超过七天的事件', async () => {
    const sdk = await load()
    sdk.trackPageView(detail)
    vi.setSystemTime(Date.now() + 8 * 86400000)
    for (let i = 0; i < 54; i++) sdk.trackPageView(home)
    await sdk.flushAnalytics()
    await vi.advanceTimersByTimeAsync(2000)
    expect(request.mock.calls.map(([payload]) => payload.data.events.length)).toEqual([50, 4])
    expect(request.mock.calls[0][0].data.events[0].page).toBe(home)
  })

  it('冷启动跳过损坏记录，前台恢复网络后重试相同的有效事件', async () => {
    const first = await load()
    first.trackPageView(home)
    const [key, saved] = [...storage.entries()][0]
    const events = saved as Array<{ event_id: string }>
    storage.set(key, [null, { ...events[0], product_id: 'broken' }, ...events])
    vi.clearAllTimers()
    vi.resetModules()
    request.mockRejectedValueOnce(new Error('offline'))
    const restored = await load()
    restored.resumeAnalytics()
    await vi.advanceTimersByTimeAsync(1500)
    expect(request.mock.calls[0][0].data.events).toEqual(events)
    await vi.advanceTimersByTimeAsync(2999)
    expect(request).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1)
    expect(request.mock.calls[1][0].data.events).toEqual(events)
    expect(storage.get(key)).toEqual([])
  })
})
