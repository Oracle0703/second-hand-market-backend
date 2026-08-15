import { apiRequest } from './request'

declare const __MERCHANT_NO__: string

type RawCategoryItem = {
  id?: number
  ID?: number
  parent_id?: number
  ParentID?: number
  level?: number
  Level?: number
  name?: string
  Name?: string
  sort?: number
  Sort?: number
}

export type BuyerProduct = {
  id: number
  title: string
  description?: string
  price_cent: number
  original_price_cent?: number | null
  stock: number
  condition_level: string
  cover_url: string
  status: string
  merchant_id: number
  merchant_name: string
  is_favorited: boolean
  can_submit_intent?: boolean
}

export type BuyerFavoriteItem = {
  product_id: number
  title: string
  cover_url: string
  price_cent: number
  original_price_cent?: number | null
  stock: number
  status: string
  favorited_at: string
}

export type BuyerHistoryItem = {
  product_id: number
  title: string
  cover_url: string
  price_cent: number
  original_price_cent?: number | null
  stock: number
  status: string
  last_viewed_at: string
  view_count: number
}

export type BuyerIntent = {
  id: number
  intent_no: string
  status: string
  buyer_status_text: string
  created_at: string
  updated_at: string
  product: { id: number; title: string; cover_url: string }
}

function merchantNo(): string {
  return (typeof __MERCHANT_NO__ === 'string' && __MERCHANT_NO__.trim()) || ''
}

function withMerchantData(params: Record<string, unknown> = {}) {
  return { ...params, merchant_no: merchantNo() }
}

function withMerchantPath(path: string) {
  const separator = path.includes('?') ? '&' : '?'
  return `${path}${separator}merchant_no=${encodeURIComponent(merchantNo())}`
}

export function fetchBuyerProducts(params: Record<string, unknown>) {
  return apiRequest<{ items: BuyerProduct[]; total: number; page: number; page_size: number }>({
    method: 'GET',
    path: '/buyer/products',
    data: withMerchantData(params)
  })
}

export function fetchBuyerProductDetail(id: string | number) {
  return apiRequest<{ product: BuyerProduct & { images: string[]; merchant: { id: number; name: string } } }>({
    method: 'GET',
    path: withMerchantPath(`/buyer/products/${id}`)
  })
}

export function fetchBuyerCategories(params: Record<string, unknown> = {}) {
  return apiRequest<{ items: RawCategoryItem[] }>({
    method: 'GET',
    path: '/buyer/categories',
    data: withMerchantData(params)
  }).then((result) => ({
    items: (result.items || []).map((item) => ({
      id: item.id ?? item.ID ?? 0,
      parent_id: item.parent_id ?? item.ParentID,
      level: item.level ?? item.Level ?? 0,
      name: item.name ?? item.Name ?? '',
      sort: item.sort ?? item.Sort ?? 0
    }))
  }))
}

export function loginByMiniProgram(payload: {
  provider: 'wechat' | 'douyin'
  code: string
  device_id: string
  nickname?: string
  avatar_url?: string
}) {
  return apiRequest<{ access_token: string; refresh_token: string; expires_in: number; user: any }>({
    method: 'POST',
    path: '/buyer/auth/miniapp-login',
    data: payload,
    skipAuth: true
  })
}

export function logoutBuyer() {
  return apiRequest<{ success: boolean }>({ method: 'POST', path: '/buyer/auth/logout', data: {} })
}

export function mergeGuest(deviceID: string) {
  return apiRequest<{ merged: { favorites_count: number; histories_count: number }; merged_at: string }>({
    method: 'POST',
    path: '/buyer/guest/merge',
    data: { device_id: deviceID }
  })
}

export function listFavorites(page = 1, pageSize = 20) {
  return apiRequest<{ items: BuyerFavoriteItem[]; total: number; page: number; page_size: number }>({
    method: 'GET',
    path: '/buyer/favorites',
    data: withMerchantData({ page, page_size: pageSize })
  })
}

export function addFavorite(productID: number) {
  return apiRequest<{ product_id: number; is_favorited: boolean }>({
    method: 'POST',
    path: withMerchantPath('/buyer/favorites'),
    data: { product_id: productID }
  })
}

export function removeFavorite(productID: number) {
  return apiRequest<{ product_id: number; is_favorited: boolean }>({
    method: 'DELETE',
    path: withMerchantPath(`/buyer/favorites/${productID}`)
  })
}

export function reportView(productID: number) {
  return apiRequest<{ product_id: number; last_viewed_at: string; view_count: number }>({
    method: 'POST',
    path: withMerchantPath('/buyer/histories/views'),
    data: { product_id: productID }
  })
}

export function listHistories(page = 1, pageSize = 20) {
  return apiRequest<{ items: BuyerHistoryItem[]; total: number; page: number; page_size: number }>({
    method: 'GET',
    path: '/buyer/histories',
    data: withMerchantData({ page, page_size: pageSize })
  })
}

export function clearHistories(productID?: number) {
  return apiRequest<{ success: boolean }>({
    method: 'DELETE',
    path: withMerchantPath('/buyer/histories'),
    data: productID ? { product_id: productID } : undefined
  })
}

export function createIntent(payload: {
  product_id: number
  contact_name?: string
  contact_phone?: string
  contact_wechat?: string
  message?: string
}) {
  return apiRequest<{ intent_id: number; intent_no: string; status: string; created_at: string }>({
    method: 'POST',
    path: withMerchantPath('/buyer/intents'),
    data: payload
  })
}

export function listIntents(params: Record<string, unknown> = {}) {
  return apiRequest<{ items: BuyerIntent[]; total: number; page: number; page_size: number }>({
    method: 'GET',
    path: '/buyer/intents',
    data: withMerchantData(params)
  })
}

export function fetchSummary() {
  return apiRequest<{
    is_login: boolean
    profile: { buyer_id?: number; nickname?: string; avatar_url?: string }
    counters: { favorites: number; histories: number; intents_open: number }
  }>({
    method: 'GET',
    path: '/buyer/me/summary'
  })
}
