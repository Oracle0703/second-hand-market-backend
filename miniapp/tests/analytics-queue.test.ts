import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { TrackEventOptions } from '../src/services/analytics'

const storage = new Map<string, unknown>()
const request = vi.fn()
const externalRequest = vi.fn()
const currentInstance = vi.fn()
let env: 'weapp' | 'tt'
let onShow: () => void

vi.mock('@tarojs/taro', () => ({
  ENV_TYPE: { WEAPP: 'weapp', TT: 'tt' },
  useDidShow: (callback: () => void) => { onShow = callback },
  default: {
    request: externalRequest,
    getCurrentInstance: currentInstance,
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
    currentInstance.mockReset()
    externalRequest.mockReset()
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

  it('配置独立平台时保留商户上报，并将商品编号和动态属性发送到项目专属队列', async () => {
    vi.stubGlobal('__EVENT_ANALYTICS_ENDPOINT__', 'https://analytics.example.test/api/events')
    vi.stubGlobal('__EVENT_ANALYTICS_PROJECT_KEY__', 'fixture_project')
    externalRequest.mockImplementation(options => {
      options.success({ statusCode: 200, data: { data: { received: options.data.events.length, discarded: 0 } } })
    })
    const sdk = await load()
    expect(sdk.trackEvent('cart_opened', { page: detail, productId: 15, properties: { quantity: 2 } })).toBe(true)
    await sdk.flushAnalytics()
    expect(request.mock.calls[0][0].data.events[0]).toMatchObject({ name: 'cart_opened', product_id: 15, properties: { quantity: 2 } })
    expect(externalRequest).toHaveBeenCalledTimes(1)
    expect(externalRequest.mock.calls[0][0]).toMatchObject({
      url: 'https://analytics.example.test/api/events',
      header: { 'X-Project-Key': 'fixture_project' },
      data: { platform: 'weapp', events: [{ name: 'cart_opened', page: detail, properties: { quantity: 2, product_id: 15 } }] }
    })
    expect(externalRequest.mock.calls[0][0].data).not.toHaveProperty('merchant_no')
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

  it.each(['weapp', 'tt'] as const)('%s 无需登记下单事件，自动取页面且剔除参数，属性在调用时快照', async (platform) => {
    env = platform
    currentInstance.mockReturnValue({ router: { path: '/orders/checkout/index?token=private#payment' } })
    const sdk = await load()
    const properties = { amount_cent: 89900, delivery: '自提', first_order: true }
    expect(sdk.trackEvent('order_submitted', { source: 'checkout_submit', productId: 7, properties })).toBe(true)
    properties.amount_cent = 0
    currentInstance.mockReturnValue(undefined)
    expect(sdk.trackEvent('inventory_synced')).toBe(true)
    sdk.trackClick('delivery_option_selected', 'orders/checkout/index', 'pickup_tab', 7)
    expect(sdk.trackEvent('page_view')).toBe(false)
    await sdk.flushAnalytics()
    const { data } = request.mock.calls[0][0]
    expect(data.platform).toBe(platform)
    expect(data.events).toHaveLength(3)
    expect(data.events[0]).toMatchObject({
      name: 'order_submitted', page: 'orders/checkout/index', source: 'checkout_submit', product_id: 7,
      properties: { amount_cent: 89900, delivery: '自提', first_order: true }
    })
    expect(data.events[1]).toMatchObject({ name: 'inventory_synced' })
    expect(data.events[1]).not.toHaveProperty('page')
    expect(data.events[2]).toMatchObject({
      name: 'delivery_option_selected', page: 'orders/checkout/index', source: 'pickup_tab', product_id: 7
    })
    expect(JSON.stringify(data.events)).not.toContain('private')
  })

  it('无效或敏感事件不会进入缓存或挤掉其他事件', async () => {
    const sdk = await load()
    for (const name of ['OrderSubmitted', 'order__submitted', 'a'.repeat(65), '']) {
      expect(sdk.trackEvent(name)).toBe(false)
    }
    const invalid: unknown[] = [
      { source: 'wrong source' }, { page: '/checkout/index?email=test' }, { productId: 0 },
      { properties: { cart_item: { count: 1 } } }, { properties: { amounts: [1] } },
      { properties: { amount: null } }, { properties: { amount: Infinity } },
      { properties: { amount: Number.MAX_SAFE_INTEGER + 1 } }, { properties: { status: 'OK\n' } },
      { properties: { status: '📦'.repeat(129) } }, { properties: { ['a'.repeat(49)]: true } },
      { properties: Object.fromEntries(Array.from({ length: 17 }, (_, i) => [`k${i}`, i])) },
      ...['session', 'billing_address', 'buyer_open_id', 'api_key_hint', 'access_token', 'user_mobile_mask']
        .map((key) => ({ properties: { [key]: 'private' } }))
    ]
    for (const options of invalid) {
      expect(sdk.trackEvent('order_submitted', options as TrackEventOptions)).toBe(false)
    }
    expect(storage.size).toBe(0)
    expect(sdk.trackEvent('order_submitted', { properties: { package_label: '📦'.repeat(128) } })).toBe(true)
    sdk.trackPageView(home, NaN)
    await sdk.flushAnalytics()
    const events = request.mock.calls[0][0].data.events
    expect(events).toHaveLength(2)
    expect(events[1]).toMatchObject({ name: 'page_view', page: home })
    expect(events[1]).not.toHaveProperty('product_id')
  })

  it('冷启动恢复旧格式和未登记事件，丢失回执后以相同 ID 和属性补报', async () => {
    const sdk = await load()
    sdk.trackPageView(home)
    const [key, saved] = [...storage.entries()][0]
    const [legacy] = saved as Record<string, unknown>[]
    const custom = {
      event_id: 'offline_order_1', occurred_at: new Date().toISOString(), name: 'order_fulfilled',
      properties: { stage: 'fulfilled', items: 2, pickup: false }
    }
    storage.set(key, [legacy, custom, custom, { ...custom, event_id: 'unsafe', properties: { customer_phone: 'private' } }])
    vi.clearAllTimers()
    vi.resetModules()
    const restored = await load()
    request.mockRejectedValueOnce(new Error('response lost'))
    await restored.flushAnalytics()
    const first = request.mock.calls[0][0].data.events
    expect(first).toEqual([legacy, custom])
    expect(storage.get(key)).toEqual(first)
    vi.clearAllTimers()
    vi.resetModules()
    await (await load()).flushAnalytics()
    expect(request.mock.calls[1][0].data.events).toEqual(first)
    expect(storage.get(key)).toEqual([])
  })
})
