import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, waitFor } from '@testing-library/react'
import { message } from 'antd'
import type { ReactNode } from 'react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { CreatePage } from './CreatePage'

const mockCategories = vi.fn()
const mockPresign = vi.fn()
const mockUploadFile = vi.fn()
const mockCreateProduct = vi.fn()

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
    presign: (...args: unknown[]) => mockPresign(...args),
    uploadFile: (...args: unknown[]) => mockUploadFile(...args),
    createProduct: (...args: unknown[]) => mockCreateProduct(...args)
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
      <MemoryRouter>
        <CreatePage />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('CreatePage upload boundary', () => {
  beforeEach(() => {
    mockCategories.mockReset().mockResolvedValue({ data: { data: { items: [] } } })
    mockPresign.mockReset()
    mockUploadFile.mockReset()
    mockCreateProduct.mockReset()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('rejects an oversize image before presign', async () => {
    const messageError = vi.spyOn(message, 'error').mockImplementation(() => undefined as never)
    const { container } = renderPage()
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
