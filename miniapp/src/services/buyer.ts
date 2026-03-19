import { apiRequest } from './request'

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

export function fetchBuyerProducts(params: Record<string, unknown>) {
  return apiRequest<{ items: BuyerProduct[]; total: number; page: number; page_size: number }>({
    method: 'GET',
    path: '/buyer/products',
    data: params
  })
}

export function fetchBuyerProductDetail(id: string | number) {
  return apiRequest<{ product: BuyerProduct & { images: string[]; merchant: { id: number; name: string } } }>({
    method: 'GET',
    path: `/buyer/products/${id}`
  })
}

export function fetchBuyerCategories(params: Record<string, unknown> = {}) {
  return apiRequest<{ items: Array<{ id: number; parent_id?: number; level: number; name: string; sort: number }> }>({
    method: 'GET',
    path: '/buyer/categories',
    data: params
  })
}

export function loginByWechat(payload: { code: string; device_id: string; nickname?: string; avatar_url?: string }) {
  return apiRequest<{ access_token: string; refresh_token: string; expires_in: number; user: any }>({
    method: 'POST',
    path: '/buyer/auth/wechat-login',
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
    data: { page, page_size: pageSize }
  })
}

export function addFavorite(productID: number) {
  return apiRequest<{ product_id: number; is_favorited: boolean }>({
    method: 'POST',
    path: '/buyer/favorites',
    data: { product_id: productID }
  })
}

export function removeFavorite(productID: number) {
  return apiRequest<{ product_id: number; is_favorited: boolean }>({
    method: 'DELETE',
    path: `/buyer/favorites/${productID}`
  })
}

export function reportView(productID: number) {
  return apiRequest<{ product_id: number; last_viewed_at: string; view_count: number }>({
    method: 'POST',
    path: '/buyer/histories/views',
    data: { product_id: productID }
  })
}

export function listHistories(page = 1, pageSize = 20) {
  return apiRequest<{ items: BuyerHistoryItem[]; total: number; page: number; page_size: number }>({
    method: 'GET',
    path: '/buyer/histories',
    data: { page, page_size: pageSize }
  })
}

export function clearHistories(productID?: number) {
  return apiRequest<{ success: boolean }>({
    method: 'DELETE',
    path: '/buyer/histories',
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
    path: '/buyer/intents',
    data: payload
  })
}

export function listIntents(params: Record<string, unknown> = {}) {
  return apiRequest<{ items: BuyerIntent[]; total: number; page: number; page_size: number }>({
    method: 'GET',
    path: '/buyer/intents',
    data: params
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
