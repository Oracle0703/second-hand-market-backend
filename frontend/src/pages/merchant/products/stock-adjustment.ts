import type { ProductStatus } from '@/constants/status'

export type StockAdjustmentType = 'INCREASE' | 'DECREASE' | 'MARK_SOLD'

export const STOCK_ADJUSTMENT_TYPE_OPTIONS: Array<{ label: string; value: StockAdjustmentType }> = [
  { label: '补充库存', value: 'INCREASE' },
  { label: '减少库存', value: 'DECREASE' },
  { label: '线下售出', value: 'MARK_SOLD' }
]

export function canAdjustProductStock(status: ProductStatus) {
  return status === 'DRAFT' || status === 'ON_SHELF' || status === 'OFF_SHELF'
}
