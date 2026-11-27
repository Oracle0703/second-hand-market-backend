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

function mockOrderDetail(product: Record<string, unknown> | null, productMissing: boolean) {
  mockedApi.orderDetail.mockResolvedValue({
    data: {
      data: {
        order_detail: { id: 7, order_no: 'O7', status: 'COMPLETED', deal_price_cent: 9900, product, product_missing: productMissing },
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

  it('links to an existing product', async () => {
    mockOrderDetail({ id: 3, title: '格力空调', status: 'SOLD', is_deleted: false }, false)
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

  it('renders a missing product without a link', async () => {
    mockOrderDetail(null, true)
    renderPage()

    expect(await screen.findByText('已删除')).toBeInTheDocument()
    expect(screen.getByText(/商品缺失/)).toBeInTheDocument()
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })
})
