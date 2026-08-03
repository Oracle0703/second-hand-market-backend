import { describe, expect, it } from 'vitest'
import type { ProductStatus } from '@/constants/status'
import { canAdjustProductStock, STOCK_ADJUSTMENT_TYPE_OPTIONS } from './stock-adjustment'

describe('stock adjustment helpers', () => {
  it('allows only draft, on-shelf, and off-shelf products to be adjusted', () => {
    const allowed: ProductStatus[] = ['DRAFT', 'ON_SHELF', 'OFF_SHELF']
    const denied: ProductStatus[] = ['LOCKED', 'SOLD', 'CLOSED']

    for (const status of allowed) {
      expect(canAdjustProductStock(status)).toBe(true)
    }
    for (const status of denied) {
      expect(canAdjustProductStock(status)).toBe(false)
    }
  })

  it('exposes the three supported stock adjustment types', () => {
    expect(STOCK_ADJUSTMENT_TYPE_OPTIONS.map((item) => item.value)).toEqual(['INCREASE', 'DECREASE', 'MARK_SOLD'])
  })
})
