import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '@/services/api'
import { EditPage } from './EditPage'

vi.mock('@/services/api', () => ({
  api: {
    categories: vi.fn(),
    productDetail: vi.fn(),
    updateProduct: vi.fn(),
    presign: vi.fn(),
    uploadFile: vi.fn()
  }
}))

const baseProduct = {
  id: 9,
  title: '旧空调',
  description: '九成新',
  status: 'DRAFT' as const,
  category_id: 21,
  price_cent: 120000,
  stock: 1,
  condition_level: 'GOOD' as const,
  images: [101],
  image_urls: ['/uploads/product_image/a.jpg']
}

function renderEditPage() {
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter initialEntries={['/merchant/products/9/edit']} future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
        <Routes>
          <Route path="/merchant/products/:productId/edit" element={<EditPage />} />
          <Route path="/merchant/products/:productId" element={<p>详情页</p>} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>
  )
}

async function submitAndGetPayload() {
  await waitFor(() => expect(screen.getByLabelText('标题')).toHaveValue('旧空调'))
  fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
  await waitFor(() => expect(api.updateProduct).toHaveBeenCalledTimes(1))
  return vi.mocked(api.updateProduct).mock.calls[0][1] as Record<string, unknown>
}

describe('商家商品编辑页原价', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(api.categories).mockImplementation(async (level?: number) => ({
      data: {
        data: {
          items: level === 1 ? [{ id: 2, name: '家电' }] : [{ id: 21, name: '空调', parent_id: 2 }]
        }
      }
    }) as never)
    vi.mocked(api.updateProduct).mockResolvedValue({ data: { data: {} } } as never)
  })

  it('原价为空时不回填现价，保存时也不提交 original_price_cent', async () => {
    vi.mocked(api.productDetail).mockResolvedValue({
      data: { data: { product: { ...baseProduct, original_price_cent: null } } }
    } as never)

    renderEditPage()

    await waitFor(() => expect(screen.getByLabelText('价格(元)')).toHaveValue('1200.00'))
    expect(screen.getByLabelText('原价(元)')).toHaveValue('')

    const payload = await submitAndGetPayload()
    expect(payload).not.toHaveProperty('original_price_cent')
    expect(payload).toMatchObject({ title: '旧空调', price_cent: 120000 })
  })

  it('原价为空时用户填写原价才提交 original_price_cent', async () => {
    vi.mocked(api.productDetail).mockResolvedValue({
      data: { data: { product: { ...baseProduct, original_price_cent: null } } }
    } as never)

    renderEditPage()

    await waitFor(() => expect(screen.getByLabelText('价格(元)')).toHaveValue('1200.00'))
    const originalInput = screen.getByLabelText('原价(元)')
    fireEvent.change(originalInput, { target: { value: '1500' } })
    fireEvent.blur(originalInput)

    const payload = await submitAndGetPayload()
    expect(payload.original_price_cent).toBe(150000)
  })

  it('已有原价且未修改时不重复提交 original_price_cent', async () => {
    vi.mocked(api.productDetail).mockResolvedValue({
      data: { data: { product: { ...baseProduct, original_price_cent: 200000 } } }
    } as never)

    renderEditPage()

    await waitFor(() => expect(screen.getByLabelText('原价(元)')).toHaveValue('2000.00'))

    const payload = await submitAndGetPayload()
    expect(payload).not.toHaveProperty('original_price_cent')
  })

  it('已有原价且用户修改后提交新的 original_price_cent', async () => {
    vi.mocked(api.productDetail).mockResolvedValue({
      data: { data: { product: { ...baseProduct, original_price_cent: 200000 } } }
    } as never)

    renderEditPage()

    await waitFor(() => expect(screen.getByLabelText('原价(元)')).toHaveValue('2000.00'))
    const originalInput = screen.getByLabelText('原价(元)')
    fireEvent.change(originalInput, { target: { value: '1800' } })
    fireEvent.blur(originalInput)

    const payload = await submitAndGetPayload()
    expect(payload.original_price_cent).toBe(180000)
  })
})
