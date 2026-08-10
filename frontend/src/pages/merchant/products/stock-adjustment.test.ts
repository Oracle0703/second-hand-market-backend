import { describe, expect, it } from 'vitest'
import type { ProductStatus } from '@/constants/status'
import { canAdjustProductStock, getStockAdjustmentTypeOptions } from './stock-adjustment'

describe('stock adjustment helpers', () => {
  it('allows draft, on-shelf, off-shelf, and sold products to be adjusted', () => {
    const allowed: ProductStatus[] = ['DRAFT', 'ON_SHELF', 'OFF_SHELF', 'SOLD']
    const denied: ProductStatus[] = ['LOCKED']

    for (const status of allowed) {
      expect(canAdjustProductStock(status)).toBe(true)
    }
    for (const status of denied) {
      expect(canAdjustProductStock(status)).toBe(false)
    }
  })

  it('derives supported stock adjustment types from product status', () => {
    expect(getStockAdjustmentTypeOptions('DRAFT').map((item) => item.value)).toEqual(['INCREASE', 'DECREASE'])
    expect(getStockAdjustmentTypeOptions('ON_SHELF').map((item) => item.value)).toEqual(['INCREASE', 'DECREASE', 'MARK_SOLD'])
    expect(getStockAdjustmentTypeOptions('OFF_SHELF').map((item) => item.value)).toEqual(['INCREASE', 'DECREASE', 'MARK_SOLD'])
    expect(getStockAdjustmentTypeOptions('SOLD').map((item) => item.value)).toEqual(['INCREASE'])
    expect(getStockAdjustmentTypeOptions('LOCKED')).toEqual([])
  })

  it('does not expose mark sold for draft products', () => {
    expect(getStockAdjustmentTypeOptions('DRAFT').map((item) => item.value)).not.toContain('MARK_SOLD')
  })
})
