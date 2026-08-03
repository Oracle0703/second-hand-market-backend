import '@testing-library/jest-dom/vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { StockAdjustmentModal } from './StockAdjustmentModal'

vi.mock('@/services/api', () => ({
  api: {
    adjustProductStock: vi.fn(async () => ({
      data: {
        data: {
          product_id: 1,
          movement_id: 9,
          adjustment_type: 'INCREASE',
          quantity: 2,
          stock_before: 1,
          stock_after: 3,
          status_before: 'ON_SHELF',
          status_after: 'ON_SHELF',
          adjusted_at: '2026-08-03T18:30:00+08:00'
        }
      }
    }))
  }
}))

const originalGetComputedStyle = window.getComputedStyle
vi.stubGlobal('getComputedStyle', (element: Element, pseudoElt?: string | null) => {
  if (pseudoElt) {
    return { getPropertyValue: () => '' } as CSSStyleDeclaration
  }
  return originalGetComputedStyle(element)
})

describe('StockAdjustmentModal', () => {
  it('shows current stock and submits an increase adjustment', async () => {
    const onSuccess = vi.fn()
    render(
      <StockAdjustmentModal
        open
        product={{ id: 1, title: '测试商品', status: 'ON_SHELF', stock: 1 }}
        onCancel={() => undefined}
        onSuccess={onSuccess}
      />
    )

    expect(screen.getByText('当前库存：1')).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('调整数量'), { target: { value: '2' } })
    fireEvent.change(screen.getByLabelText('调整原因'), { target: { value: '盘点补录' } })
    fireEvent.click(screen.getByRole('button', { name: '确认调整' }))

    await waitFor(() => expect(onSuccess).toHaveBeenCalledTimes(1))
  })
})
