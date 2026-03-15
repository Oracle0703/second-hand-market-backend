export type ProductStatus = 'DRAFT' | 'ON_SHELF' | 'LOCKED' | 'OFF_SHELF' | 'SOLD' | 'CLOSED'
export type OrderStatus = 'CREATED' | 'COMPLETED' | 'CLOSED'
export type IntentStatus = 'NEW' | 'CONTACTED' | 'CLOSED'
export type MerchantReviewStatus = 'PENDING' | 'APPROVED' | 'REJECTED'
export type ProductCondition = 'LIKE_NEW' | 'GOOD' | 'FAIR' | 'POOR'

type StatusMeta = {
  text: string
  color?: string
}

export const PRODUCT_STATUS_META: Record<ProductStatus, StatusMeta> = {
  DRAFT: { text: '草稿', color: 'default' },
  ON_SHELF: { text: '在售', color: 'success' },
  LOCKED: { text: '锁定', color: 'gold' },
  OFF_SHELF: { text: '下架', color: 'processing' },
  SOLD: { text: '已成交', color: 'blue' },
  CLOSED: { text: '已关闭', color: 'default' }
}

export const ORDER_STATUS_META: Record<OrderStatus, StatusMeta> = {
  CREATED: { text: '待处理', color: 'processing' },
  COMPLETED: { text: '已完成', color: 'success' },
  CLOSED: { text: '已关闭', color: 'default' }
}

export const INTENT_STATUS_META: Record<IntentStatus, StatusMeta> = {
  NEW: { text: '待联系', color: 'processing' },
  CONTACTED: { text: '已联系', color: 'success' },
  CLOSED: { text: '已关闭', color: 'default' }
}

export const MERCHANT_REVIEW_STATUS_META: Record<MerchantReviewStatus, StatusMeta> = {
  PENDING: { text: '待审核', color: 'processing' },
  APPROVED: { text: '已通过', color: 'success' },
  REJECTED: { text: '已驳回', color: 'error' }
}

export const ACCOUNT_STATUS_META: Record<string, StatusMeta> = {
  ACTIVE: { text: '正常', color: 'success' },
  DISABLED: { text: '已禁用', color: 'error' }
}

export const PRODUCT_CONDITION_META: Record<ProductCondition, StatusMeta> = {
  LIKE_NEW: { text: '近乎全新' },
  GOOD: { text: '成色良好' },
  FAIR: { text: '成色一般' },
  POOR: { text: '成色较差' }
}

const COMMON_STATUS_META: Record<string, StatusMeta> = {
  ...PRODUCT_STATUS_META,
  ...ORDER_STATUS_META,
  ...INTENT_STATUS_META,
  ...MERCHANT_REVIEW_STATUS_META,
  ...ACCOUNT_STATUS_META
}

export function toValueEnum<T extends string>(meta: Record<T, StatusMeta>) {
  const out: Partial<Record<T, { text: string }>> = {}
  for (const key of Object.keys(meta) as T[]) {
    out[key] = { text: meta[key].text }
  }
  return out as Record<T, { text: string }>
}

export function getStatusText(meta: Record<string, StatusMeta>, value?: string | null, fallback = '-') {
  if (!value) return fallback
  return meta[value]?.text ?? value
}

export function getStatusColor(meta: Record<string, StatusMeta>, value?: string | null, fallback = 'default') {
  if (!value) return fallback
  return meta[value]?.color ?? fallback
}

export function getCommonStatusText(value?: string | null, fallback = '-') {
  return getStatusText(COMMON_STATUS_META, value, fallback)
}
