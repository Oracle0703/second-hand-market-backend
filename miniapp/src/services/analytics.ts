import Taro from '@tarojs/taro'
import { merchantNo, withMerchantPath } from './buyer'
import { apiRequest } from './request'
import { resolveTaroEnv } from '../utils/platform'

declare const __API_BASE_URL__: string

export type ClickName = string
export type AnalyticsProperties = Record<string, string | number | boolean>
export interface TrackEventOptions {
  page?: string
  source?: string
  productId?: number
  properties?: AnalyticsProperties
}

type EventFields = {
  name: string
  page?: string
  source?: string
  product_id?: number
  properties?: AnalyticsProperties
}
type AnalyticsEvent = EventFields & {
  event_id: string
  occurred_at: string
}

const batchSize = 50
const maxQueueSize = 200
const maxAge = 7 * 24 * 60 * 60 * 1000
const flushDelay = 1500
const identifier = /^[a-z][a-z0-9]*(_[a-z0-9]+)*$/
const route = /^[A-Za-z0-9_-]+(\/[A-Za-z0-9_-]+)+$/
const sensitiveKey = /(?:^|_)(?:token|password|passwd|secret|credential|authorization|cookie|phone|mobile|email|openid|unionid|address|idcard|session|id_card|open_id|union_id|access_key|api_key)(?:_|$)/
let pending: AnalyticsEvent[] | undefined
let storageKey: string
let sequence = 0
let timer: ReturnType<typeof setTimeout> | undefined
let sending: Promise<void> | undefined
let retryDelay = 0
let visible = true

function isRecent(occurredAt: string, now: number) {
  const time = Date.parse(occurredAt)
  return time >= now - maxAge && time <= now + 10 * 60 * 1000
}

function validIdentifier(value: unknown, limit: number): value is string {
  return typeof value === 'string' && value.length <= limit && identifier.test(value)
}

function validProductId(value: unknown): value is number {
  return typeof value === 'number' && Number.isSafeInteger(value) && value > 0
}

function validProperties(value: unknown): value is AnalyticsProperties {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) return false
  const prototype = Object.getPrototypeOf(value)
  if (prototype !== null && Object.getPrototypeOf(prototype) !== null) return false
  const entries = Object.entries(value)
  if (entries.length > 16) return false
  return entries.every(([key, item]) => {
    if (!validIdentifier(key, 48) || sensitiveKey.test(key)) return false
    if (typeof item === 'string') {
      return item.length <= 256 && Array.from(item).length <= 128 && !/[\u0000-\u001f\u007f-\u009f]/.test(item)
    }
    if (typeof item === 'number') return Number.isFinite(item) && Math.abs(item) <= Number.MAX_SAFE_INTEGER
    return typeof item === 'boolean'
  })
}

function eventFields(name: unknown, options: TrackEventOptions): EventFields | undefined {
  const { page, source, productId, properties } = options
  if (!validIdentifier(name, 64)) return
  if (page !== undefined && page !== '' && (typeof page !== 'string' || page.length > 128 || !route.test(page))) return
  if (name === 'page_view' && !page) return
  if (source !== undefined && source !== '' && !validIdentifier(source, 64)) return
  if (productId !== undefined && !validProductId(productId)) return
  if (properties !== undefined && !validProperties(properties)) return
  return {
    name,
    ...(page ? { page } : {}),
    ...(source ? { source } : {}),
    ...(productId !== undefined ? { product_id: productId } : {}),
    ...(properties ? { properties: { ...properties } } : {})
  }
}

function restoreEvent(item: unknown, now: number): AnalyticsEvent | undefined {
  if (!item || typeof item !== 'object') return
  const { event_id, occurred_at, name, page, source, product_id, properties } = item as Partial<AnalyticsEvent>
  if (typeof event_id !== 'string' || !/^[a-zA-Z0-9_-]{1,64}$/.test(event_id) ||
      typeof occurred_at !== 'string' || !isRecent(occurred_at, now)) return
  const fields = eventFields(name, { page, source, productId: product_id, properties })
  if (fields) return { ...fields, event_id, occurred_at }
}

function queue(): AnalyticsEvent[] {
  if (pending) return pending
  const api = typeof __API_BASE_URL__ === 'string' ? __API_BASE_URL__ : ''
  storageKey = `buyer_analytics_v1:${api}:${merchantNo()}:${resolveTaroEnv()}`
  try {
    const saved: unknown = Taro.getStorageSync(storageKey)
    const now = Date.now()
    pending = []
    if (Array.isArray(saved)) {
      const restoredIds = new Set<string>()
      for (const item of saved.slice(-maxQueueSize)) {
        const event = restoreEvent(item, now)
        if (event && !restoredIds.has(event.event_id)) {
          restoredIds.add(event.event_id)
          pending.push(event)
        }
      }
      if (saved.length) persist()
    }
  } catch {
    pending = []
  }
  return pending
}

function persist() {
  try {
    Taro.setStorageSync(storageKey, pending)
  } catch {
    // The in-memory batch can still be sent when storage is unavailable.
  }
}

function schedule() {
  if (!visible || sending || timer || !queue().length) return
  timer = setTimeout(() => {
    timer = undefined
    void flushAnalytics()
  }, retryDelay || flushDelay)
}

async function sendBatch(events: AnalyticsEvent[]) {
  try {
    await apiRequest({
      method: 'POST',
      path: withMerchantPath('/buyer/analytics/events'),
      skipAuth: true,
      data: { platform: resolveTaroEnv(), events }
    })
    const acknowledged = new Set(events.map((event) => event.event_id))
    pending = queue().filter((event) => !acknowledged.has(event.event_id))
    persist()
    retryDelay = 0
  } catch {
    retryDelay = Math.min((retryDelay || flushDelay) * 2, 60000)
  }
}

export function flushAnalytics(): Promise<void> {
  clearTimeout(timer)
  timer = undefined
  if (sending) return sending
  const now = Date.now()
  const entries = queue()
  const valid = entries.filter((event) => isRecent(event.occurred_at, now))
  if (valid.length !== entries.length) {
    pending = valid
    persist()
  }
  if (!valid.length) return Promise.resolve()
  sending = sendBatch(valid.slice(0, batchSize)).finally(() => {
    sending = undefined
    schedule()
  })
  return sending
}

function currentRoute(): string | undefined {
  try {
    const path = Taro.getCurrentInstance()?.router?.path?.split(/[?#]/, 1)[0].replace(/^\//, '')
    if (path && path.length <= 128 && route.test(path)) return path
  } catch {
    // A background event can be recorded before a page instance exists.
  }
}

/** Returns true when the event has entered the local queue. */
export function trackEvent(name: string, options: TrackEventOptions = {}): boolean {
  if (!options || typeof options !== 'object' || Array.isArray(options)) return false
  const fields = eventFields(name, { ...options, page: options.page === undefined ? currentRoute() : options.page })
  if (!fields) return false
  const events = queue()
  const now = new Date()
  events.push({
    ...fields,
    event_id: `e_${now.getTime().toString(36)}_${Math.random().toString(36).slice(2, 12)}_${(++sequence).toString(36)}`,
    occurred_at: now.toISOString()
  })
  if (events.length > maxQueueSize) events.shift()
  persist()
  schedule()
  return true
}

export function trackPageView(page: string, productId?: number) {
  trackEvent('page_view', { page, productId: validProductId(productId) ? productId : undefined })
}

export function trackClick(name: ClickName, page: string, source: string, productId?: number) {
  trackEvent(name, { page, source, productId: validProductId(productId) ? productId : undefined })
}

export function resumeAnalytics() {
  visible = true
  schedule()
}

export function pauseAnalytics() {
  visible = false
  void flushAnalytics()
}
