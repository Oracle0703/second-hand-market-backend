import { http, type APIResponse } from './http'
import type { LoginResponse, LoginType } from '../types/auth'

type PresignResponse = {
  file_id: number
  object_key: string
  file_token?: string
  expire_at?: string
}

export const api = {
  login(payload: { login_type: LoginType; username: string; password: string }) {
    return http.post<APIResponse<LoginResponse>>('/auth/login', payload)
  },
  logout() {
    return http.post<APIResponse<{ success: boolean }>>('/auth/logout', {})
  },
  register(payload: {
    merchant_name: string
    contact_name: string
    phone: string
    username: string
    password: string
    license_file_id: number
    license_file_token: string
  }) {
    return http.post('/auth/register', payload)
  },
  merchantProfile() {
    return http.get('/merchant/profile')
  },
  merchantReapply(payload: Record<string, unknown>) {
    return http.post('/merchant/reapply', payload)
  },
  categories(level?: 1 | 2, parentId?: number) {
    const params: Record<string, string | number> = {}
    if (level) params.level = level
    if (parentId) params.parent_id = parentId
    return http.get('/merchant/categories', { params })
  },
  presign(payload: { biz_type: string; file_name: string; file_size: number; mime_type: string }) {
    return http.post<APIResponse<PresignResponse>>('/files/presign', payload)
  },
  uploadFile(formData: FormData) {
    return http.post('/files/upload', formData)
  },
  confirmUpload(payload: { file_id: number; object_key: string }) {
    return http.post('/files/confirm', payload)
  },
  products(params: Record<string, string | number> = {}) {
    return http.get('/merchant/products', { params })
  },
  productDetail(productId: string | number) {
    return http.get(`/merchant/products/${productId}`)
  },
  createProduct(payload: {
    title: string
    description: string
    category_id: number
    price_cent: number
    original_price_cent: number
    condition_level: 'LIKE_NEW' | 'GOOD' | 'FAIR' | 'POOR'
    stock: number
    image_file_ids: number[]
  }) {
    return http.post('/merchant/products', payload)
  },
  updateProduct(
    productId: string | number,
    payload: Partial<{
      title: string
      description: string
      category_id: number
      price_cent: number
      original_price_cent: number
      condition_level: 'LIKE_NEW' | 'GOOD' | 'FAIR' | 'POOR'
      stock: number
      image_file_ids: number[]
    }>
  ) {
    return http.put(`/merchant/products/${productId}`, payload)
  },
  deleteProduct(productId: string | number) {
    return http.delete(`/merchant/products/${productId}`)
  },
  productOnShelf(productId: string | number) {
    return http.post(`/merchant/products/${productId}/on-shelf`, {})
  },
  productOffShelf(productId: string | number) {
    return http.post(`/merchant/products/${productId}/off-shelf`, {})
  },
  productClose(productId: string | number, reason?: string) {
    return http.post(`/merchant/products/${productId}/close`, reason ? { reason } : {})
  },
  orders(params: Record<string, string | number> = {}) {
    return http.get('/merchant/orders', { params })
  },
  createOrder(payload: { product_id: number; quantity: number; deal_price_cent: number; buyer_contact_masked?: string; remark?: string }) {
    return http.post('/merchant/orders', payload)
  },
  orderDetail(orderId: string | number) {
    return http.get(`/merchant/orders/${orderId}`)
  },
  orderComplete(orderId: string | number, note?: string) {
    return http.post(`/merchant/orders/${orderId}/complete`, note ? { note } : {})
  },
  orderClose(orderId: string | number, reason?: string) {
    return http.post(`/merchant/orders/${orderId}/close`, reason ? { reason } : {})
  },
  merchantIntents(params: Record<string, string | number> = {}) {
    return http.get('/merchant/intents', { params })
  },
  merchantIntentDetail(intentId: string | number) {
    return http.get(`/merchant/intents/${intentId}`)
  },
  merchantIntentContacted(intentId: string | number) {
    return http.post(`/merchant/intents/${intentId}/contacted`, {})
  },
  merchantIntentClose(intentId: string | number, payload: { reason?: string; merchant_note?: string } = {}) {
    return http.post(`/merchant/intents/${intentId}/close`, payload)
  },
  adminMerchantReviews(params: Record<string, string | number> = {}) {
    return http.get('/admin/merchants', { params })
  },
  adminMerchantReviewDetail(merchantId: string | number) {
    return http.get(`/admin/merchants/${merchantId}`)
  },
  adminLicenseContent(fileID: string | number) {
    return http.get<Blob>(`/admin/files/${fileID}/content`, { responseType: 'blob' })
  },
  adminMerchantApprove(merchantId: string | number, comment?: string) {
    return http.post(`/admin/merchants/${merchantId}/approve`, comment ? { comment } : {})
  },
  adminMerchantReject(merchantId: string | number, reason: string) {
    return http.post(`/admin/merchants/${merchantId}/reject`, { reason })
  },
  adminLogs(params: Record<string, string | number> = {}) {
    return http.get('/admin/logs', { params })
  },
  adminChangePassword(payload: { current_password: string; new_password: string }) {
    return http.put('/admin/account/password', payload)
  },
  merchantLogs(params: Record<string, string | number> = {}) {
    return http.get('/merchant/logs', { params })
  },
  merchantAccount() {
    return http.get('/merchant/account')
  },
  merchantChangePassword(payload: { old_password: string; new_password: string }) {
    return http.put('/merchant/account/password', payload)
  }
}
