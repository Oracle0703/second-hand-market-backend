import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { CreateOrderModal } from './CreateOrderModal'

const mockCreateOrder = vi.fn()
const jsdomGetComputedStyle = window.getComputedStyle.bind(window)

vi.mock('@/services/api', () => ({
  api: {
    createOrder: (...args: unknown[]) => mockCreateOrder(...args)
  }
}))

describe('CreateOrderModal', () => {
  beforeEach(() => {
    mockCreateOrder.mockReset()
    vi.spyOn(window, 'getComputedStyle').mockImplementation((element) => jsdomGetComputedStyle(element))
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('calculates the total and submits quantity with the unit price', async () => {
    const onCancel = vi.fn()
    const onCreated = vi.fn()
    mockCreateOrder.mockResolvedValue({ data: { data: { order_id: 1 } } })

    render(
      <CreateOrderModal
        open
        product={{ id: 42, title: '测试茶杯', price_cent: 1234, available_stock: 5 }}
        onCancel={onCancel}
        onCreated={onCreated}
      />
    )

    await screen.findByText('订单总价：12.34 元')
    fireEvent.change(screen.getByLabelText('数量'), { target: { value: '3' } })
    fireEvent.change(screen.getByLabelText('单件成交价(元)'), { target: { value: '11.5' } })
    expect(await screen.findByText('订单总价：34.50 元')).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: /创\s*建/ }))

    await waitFor(() => {
      expect(mockCreateOrder).toHaveBeenCalledWith({
        product_id: 42,
        quantity: 3,
        deal_price_cent: 1150
      })
      expect(onCreated).toHaveBeenCalledOnce()
      expect(onCancel).toHaveBeenCalledOnce()
    })
  })
})
