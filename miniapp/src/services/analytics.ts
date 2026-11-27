import Taro from '@tarojs/taro'
import { merchantNo, withMerchantPath } from './buyer'
import { apiRequest } from './request'
import { resolveTaroEnv } from '../utils/platform'

declare const __API_BASE_URL__: string

export type ClickName = 'product_click' | 'favorite_click' | 'contact_click' | 'contact_confirm'
type AnalyticsEvent = {
  event_id: string
  name: ClickName | 'page_view'
  page: string
  source?: string
  product_id?: number
  occurred_at: string
}

const batchSize = 50
const maxQueueSize = 200
const maxAge = 7 * 24 * 60 * 60 * 1000
const flushDelay = 1500
const eventNames = new Set(['page_view', 'product_click', 'favorite_click', 'contact_click', 'contact_confirm'])
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

function isStoredEvent(item: unknown, now: number): item is AnalyticsEvent {
  if (!item || typeof item !== 'object') return false
  const event = item as Partial<AnalyticsEvent>
  return typeof event.event_id === 'string' && /^[a-zA-Z0-9_-]{1,64}$/.test(event.event_id) &&
    typeof event.page === 'string' && typeof event.name === 'string' && eventNames.has(event.name) &&
    (event.source === undefined || typeof event.source === 'string') &&
    (event.product_id === undefined || (Number.isSafeInteger(event.product_id) && event.product_id > 0)) &&
    typeof event.occurred_at === 'string' && isRecent(event.occurred_at, now)
}

function queue(): AnalyticsEvent[] {
  if (pending) return pending
  const api = typeof __API_BASE_URL__ === 'string' ? __API_BASE_URL__ : ''
  storageKey = `buyer_analytics_v1:${api}:${merchantNo()}:${resolveTaroEnv()}`
  try {
    const saved: unknown = Taro.getStorageSync(storageKey)
    const now = Date.now()
    pending = Array.isArray(saved) ? saved.filter((item) => isStoredEvent(item, now)).slice(-maxQueueSize) : []
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

function record(name: AnalyticsEvent['name'], page: string, source?: string, productId?: number) {
  const events = queue()
  const now = new Date()
  events.push({
    event_id: `e_${now.getTime().toString(36)}_${Math.random().toString(36).slice(2, 12)}_${(++sequence).toString(36)}`,
    name,
    page,
    ...(source ? { source } : {}),
    ...(productId && Number.isSafeInteger(productId) && productId > 0 ? { product_id: productId } : {}),
    occurred_at: now.toISOString()
  })
  if (events.length > maxQueueSize) events.shift()
  persist()
  schedule()
}

export function trackPageView(page: string, productId?: number) {
  record('page_view', page, undefined, productId)
}

export function trackClick(name: ClickName, page: string, source: string, productId?: number) {
  record(name, page, source, productId)
}

export function resumeAnalytics() {
  visible = true
  schedule()
}

export function pauseAnalytics() {
  visible = false
  void flushAnalytics()
}
