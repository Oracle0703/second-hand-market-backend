import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '@/services/api'
import { DetailPage } from './DetailPage'

vi.mock('@/services/api', () => ({
  api: {
    orderDetail: vi.fn(),
    orderComplete: vi.fn(),
    orderClose: vi.fn()
  }
}))

vi.mock('@ant-design/pro-components', async () => {
  const React = await vi.importActual<typeof import('react')>('react')

  return {
    PageContainer: ({ title, children }: { title: string; children?: ReactNode }) =>
      React.createElement('main', null, React.createElement('h1', null, title), children),
    ProDescriptions: ({ columns, dataSource }: { columns: Array<{ title: string; key?: string; dataIndex?: string; render?: (value: unknown, row: never) => ReactNode }>; dataSource: Record<string, unknown> }) =>
      React.createElement(
        'dl',
        null,
        columns.map((column) =>
          React.createElement(
            'div',
            { key: column.key ?? column.dataIndex },
            React.createElement('dt', null, column.title),
            React.createElement('dd', null, column.render ? column.render(column.dataIndex ? dataSource[column.dataIndex] : undefined, dataSource as never) : String(dataSource[column.dataIndex ?? ''] ?? ''))
          )
        )
      ),
    ProTable: () => null
  }
})

const mockedApi = vi.mocked(api)

function mockOrderDetail(product: Record<string, unknown> | null, productMissing?: boolean) {
  mockedApi.orderDetail.mockResolvedValue({
    data: {
      data: {
        order_detail: {
          id: 7, order_no: 'O7', status: 'COMPLETED', deal_price_cent: 9900, product,
          ...(productMissing === undefined ? {} : { product_missing: productMissing })
        },
        events: []
      }
    }
  } as never)
}

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={['/merchant/orders/7']}>
        <Routes>
          <Route path="/merchant/orders/:orderId" element={<DetailPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('Order DetailPage product cell', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it.each([true, false])('links to an existing product with API availability flags = %s', async (hasFlags) => {
    const product = { id: 3, title: '格力空调', status: 'SOLD' }
    if (hasFlags) mockOrderDetail({ ...product, is_deleted: false }, false)
    else mockOrderDetail(product)
    renderPage()

    const link = await screen.findByRole('link', { name: '格力空调' })
    expect(link).toHaveAttribute('href', '/merchant/products/3')
    expect(screen.queryByText('已删除')).not.toBeInTheDocument()
  })

  it('renders a soft-deleted product as plain text with a deleted tag', async () => {
    mockOrderDetail({ id: 3, title: '格力空调', status: 'SOLD', is_deleted: true }, false)
    renderPage()

    expect(await screen.findByText('已删除')).toBeInTheDocument()
    expect(screen.getByText(/格力空调/)).toBeInTheDocument()
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })

  it.each([
    { format: 'flagged placeholder', product: { id: 0, title: '', status: '', is_deleted: false }, missing: true },
    { format: 'legacy zero-value object', product: { id: 0, title: '', status: '' }, missing: undefined },
    { format: 'null product', product: null, missing: true }
  ])('shows a missing product without a link for a $format', async ({ product, missing }) => {
    mockOrderDetail(product, missing)
    renderPage()

    expect(await screen.findByText('商品缺失')).toBeInTheDocument()
    expect(screen.queryByText('已删除')).not.toBeInTheDocument()
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })
})
