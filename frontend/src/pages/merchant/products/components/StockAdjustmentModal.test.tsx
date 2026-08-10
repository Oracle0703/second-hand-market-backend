import '@testing-library/jest-dom/vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { api } from '@/services/api'
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
    return { getPropertyValue: () => '' } as unknown as CSSStyleDeclaration
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

  it('submits all remaining stock for the mark-sold shortcut without using the displayed stock as quantity', async () => {
    const onSuccess = vi.fn()
    render(
      <StockAdjustmentModal
        open
        product={{ id: 1, title: '测试商品', status: 'ON_SHELF', stock: 7 }}
        markSoldAllRemaining
        onCancel={() => undefined}
        onSuccess={onSuccess}
      />
    )

    fireEvent.change(screen.getByLabelText('调整原因'), { target: { value: '全部售罄' } })
    fireEvent.click(screen.getByRole('button', { name: '确认调整' }))

    await waitFor(() =>
      expect(api.adjustProductStock).toHaveBeenCalledWith(1, {
        adjustment_type: 'MARK_SOLD',
        all_remaining: true,
        reason: '全部售罄'
      })
    )
  })

  it('resets cancelled stock adjustments when reopening with a different status or mode', async () => {
    vi.clearAllMocks()
    const onCancel = vi.fn()
    const onSuccess = vi.fn()
    const { rerender } = render(
      <StockAdjustmentModal
        open
        product={{ id: 1, title: '测试商品', status: 'ON_SHELF', stock: 7 }}
        onCancel={onCancel}
        onSuccess={onSuccess}
      />
    )

    fireEvent.mouseDown(screen.getByRole('combobox'))
    fireEvent.click(screen.getByText('线下售出'))
    fireEvent.change(screen.getByLabelText('调整数量'), { target: { value: '4' } })
    fireEvent.change(screen.getByLabelText('调整原因'), { target: { value: '旧的部分售出原因' } })
    fireEvent.click(screen.getByRole('button', { name: /取\s*消/ }))
    expect(onCancel).toHaveBeenCalledTimes(1)

    rerender(
      <StockAdjustmentModal
        open={false}
        product={{ id: 1, title: '测试商品', status: 'ON_SHELF', stock: 7 }}
        onCancel={onCancel}
        onSuccess={onSuccess}
      />
    )
    rerender(
      <StockAdjustmentModal
        open
        product={{ id: 1, title: '测试商品', status: 'ON_SHELF', stock: 7 }}
        markSoldAllRemaining
        onCancel={onCancel}
        onSuccess={onSuccess}
      />
    )

    expect(screen.queryByLabelText('调整数量')).not.toBeInTheDocument()
    expect(screen.getByLabelText('调整原因')).toHaveValue('')
    fireEvent.change(screen.getByLabelText('调整原因'), { target: { value: '全部售罄' } })
    fireEvent.click(screen.getByRole('button', { name: '确认调整' }))
    await waitFor(() =>
      expect(api.adjustProductStock).toHaveBeenCalledWith(1, {
        adjustment_type: 'MARK_SOLD',
        all_remaining: true,
        reason: '全部售罄'
      })
    )

    rerender(
      <StockAdjustmentModal
        open={false}
        product={{ id: 1, title: '测试商品', status: 'ON_SHELF', stock: 7 }}
        markSoldAllRemaining
        onCancel={onCancel}
        onSuccess={onSuccess}
      />
    )
    rerender(
      <StockAdjustmentModal
        open
        product={{ id: 1, title: '测试商品', status: 'SOLD', stock: 0 }}
        onCancel={onCancel}
        onSuccess={onSuccess}
      />
    )

    expect(screen.getByLabelText('调整数量')).toHaveValue('1')
    expect(screen.getByLabelText('调整原因')).toHaveValue('')
    fireEvent.change(screen.getByLabelText('调整原因'), { target: { value: '补货' } })
    fireEvent.click(screen.getByRole('button', { name: '确认调整' }))

    await waitFor(() =>
      expect(api.adjustProductStock).toHaveBeenLastCalledWith(1, {
        adjustment_type: 'INCREASE',
        quantity: 1,
        reason: '补货'
      })
    )
  })
})
