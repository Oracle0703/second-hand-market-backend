import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { message } from 'antd'
import type { ReactNode } from 'react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { EditPage } from './EditPage'

const mockCategories = vi.fn()
const mockProductDetail = vi.fn()
const mockPresign = vi.fn()
const mockUploadFile = vi.fn()
const mockUpdateProduct = vi.fn()

vi.mock('@ant-design/pro-components', () => ({
  PageContainer: ({ children }: { children?: ReactNode }) => <main>{children}</main>,
  ProCard: ({ children }: { children?: ReactNode }) => <section>{children}</section>,
  ProForm: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
  ProFormDigit: () => null,
  ProFormSelect: () => null,
  ProFormText: () => null,
  ProFormTextArea: () => null
}))

vi.mock('@/services/api', () => ({
  api: {
    categories: (...args: unknown[]) => mockCategories(...args),
    productDetail: (...args: unknown[]) => mockProductDetail(...args),
    presign: (...args: unknown[]) => mockPresign(...args),
    uploadFile: (...args: unknown[]) => mockUploadFile(...args),
    updateProduct: (...args: unknown[]) => mockUpdateProduct(...args)
  }
}))

function renderPage() {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false }
    }
  })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={['/merchant/products/42/edit']}>
        <Routes>
          <Route path="/merchant/products/:productId/edit" element={<EditPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('EditPage upload boundary', () => {
  beforeEach(() => {
    mockCategories.mockReset().mockResolvedValue({ data: { data: { items: [] } } })
    mockProductDetail.mockReset().mockResolvedValue({
      data: {
        data: {
          product: {
            id: 42,
            title: 'Draft product',
            description: 'Draft description',
            status: 'DRAFT',
            category_id: 2,
            price_cent: 100,
            original_price_cent: 100,
            stock: 1,
            reserved_stock: 0,
            available_stock: 1,
            condition_level: 'GOOD',
            images: [],
            image_urls: []
          }
        }
      }
    })
    mockPresign.mockReset()
    mockUploadFile.mockReset()
    mockUpdateProduct.mockReset()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('rejects an oversize image before presign', async () => {
    const messageError = vi.spyOn(message, 'error').mockImplementation(() => undefined as never)
    const { container } = renderPage()
    await screen.findByText('暂无图片')
    const input = container.querySelector('input[type="file"]') as HTMLInputElement

    fireEvent.change(input, {
      target: {
        files: [new File([new Uint8Array(10 * 1024 * 1024 + 1)], 'oversize.jpg', { type: 'image/jpeg' })]
      }
    })

    await waitFor(() => {
      expect(messageError).toHaveBeenCalledWith('图片不能超过 10 MiB')
    })
    expect(mockPresign).not.toHaveBeenCalled()
    expect(mockUploadFile).not.toHaveBeenCalled()
  })
})
