import { beforeEach, describe, expect, test, vi } from 'vitest'

const mockApiRequest = vi.fn(async () => ({}))

vi.mock('../src/services/request', () => ({
  apiRequest: (options: unknown) => mockApiRequest(options)
}))

describe('buyer merchant scope', () => {
  beforeEach(() => {
    vi.resetModules()
    mockApiRequest.mockClear()
    vi.stubGlobal('__MERCHANT_NO__', 'M20260815001')
  })

  test('adds merchant_no to buyer GET request data', async () => {
    const buyer = await import('../src/services/buyer')

    await buyer.fetchBuyerProducts({ page: 1 })
    await buyer.fetchBuyerCategories({ level: 1 })
    await buyer.listFavorites()
    await buyer.listHistories()
    await buyer.listIntents({ status: 'NEW' })
    await buyer.fetchSummary()

    expect(mockApiRequest).toHaveBeenNthCalledWith(1, expect.objectContaining({
      method: 'GET',
      path: '/buyer/products',
      data: { page: 1, merchant_no: 'M20260815001' }
    }))
    expect(mockApiRequest).toHaveBeenNthCalledWith(2, expect.objectContaining({
      method: 'GET',
      path: '/buyer/categories',
      data: { level: 1, merchant_no: 'M20260815001' }
    }))
    expect(mockApiRequest).toHaveBeenNthCalledWith(3, expect.objectContaining({
      method: 'GET',
      path: '/buyer/favorites',
      data: { page: 1, page_size: 20, merchant_no: 'M20260815001' }
    }))
    expect(mockApiRequest).toHaveBeenNthCalledWith(4, expect.objectContaining({
      method: 'GET',
      path: '/buyer/histories',
      data: { page: 1, page_size: 20, merchant_no: 'M20260815001' }
    }))
    expect(mockApiRequest).toHaveBeenNthCalledWith(5, expect.objectContaining({
      method: 'GET',
      path: '/buyer/intents',
      data: { status: 'NEW', merchant_no: 'M20260815001' }
    }))
    expect(mockApiRequest).toHaveBeenNthCalledWith(6, expect.objectContaining({
      method: 'GET',
      path: '/buyer/me/summary',
      data: { merchant_no: 'M20260815001' }
    }))
  })

  test('adds merchant_no to buyer write and detail paths', async () => {
    const buyer = await import('../src/services/buyer')

    await buyer.fetchBuyerProductDetail(7)
    await buyer.addFavorite(7)
    await buyer.removeFavorite(7)
    await buyer.reportView(7)
    await buyer.clearHistories(7)
    await buyer.createIntent({ product_id: 7, contact_phone: '13800138000' })

    expect(mockApiRequest).toHaveBeenNthCalledWith(1, expect.objectContaining({
      method: 'GET',
      path: '/buyer/products/7?merchant_no=M20260815001'
    }))
    expect(mockApiRequest).toHaveBeenNthCalledWith(2, expect.objectContaining({
      method: 'POST',
      path: '/buyer/favorites?merchant_no=M20260815001',
      data: { product_id: 7 }
    }))
    expect(mockApiRequest).toHaveBeenNthCalledWith(3, expect.objectContaining({
      method: 'DELETE',
      path: '/buyer/favorites/7?merchant_no=M20260815001'
    }))
    expect(mockApiRequest).toHaveBeenNthCalledWith(4, expect.objectContaining({
      method: 'POST',
      path: '/buyer/histories/views?merchant_no=M20260815001',
      data: { product_id: 7 }
    }))
    expect(mockApiRequest).toHaveBeenNthCalledWith(5, expect.objectContaining({
      method: 'DELETE',
      path: '/buyer/histories?merchant_no=M20260815001&product_id=7'
    }))
    expect(mockApiRequest).toHaveBeenNthCalledWith(6, expect.objectContaining({
      method: 'POST',
      path: '/buyer/intents?merchant_no=M20260815001',
      data: { product_id: 7, contact_phone: '13800138000' }
    }))
  })
})
