import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '@/services/api'
import { DetailPage } from './DetailPage'
import { ListPage } from './ListPage'

vi.mock('@/services/api', () => ({
  api: {
    categories: vi.fn(),
    products: vi.fn(),
    productDetail: vi.fn(),
    productOnShelf: vi.fn(),
    productOffShelf: vi.fn(),
    createOrder: vi.fn(),
    deleteProduct: vi.fn(),
    adjustProductStock: vi.fn()
  }
}))

vi.mock('@ant-design/pro-components', async () => {
  const React = await vi.importActual<typeof import('react')>('react')

  return {
    PageContainer: ({ title, extra, children }: { title: string; extra?: ReactNode; children?: ReactNode }) =>
      React.createElement('main', null, React.createElement('h1', null, title), extra, children),
    ProCard: ({ children }: { children?: ReactNode }) => React.createElement('section', null, children),
    ProDescriptions: ({ columns, dataSource }: { columns: Array<{ title: string; dataIndex: string; render?: (value: unknown, row: never) => ReactNode }>; dataSource: Record<string, unknown> }) =>
      React.createElement(
        'dl',
        null,
        columns.map((column) =>
          React.createElement(
            'div',
            { key: column.dataIndex },
            React.createElement('dt', null, column.title),
            React.createElement('dd', null, column.render ? column.render(dataSource[column.dataIndex], dataSource as never) : String(dataSource[column.dataIndex] ?? ''))
          )
        )
      ),
    ProTable: ({ columns, request }: { columns: Array<{ key?: string; dataIndex?: string; title: string; valueEnum?: Record<string, { text: string }>; render?: (value: unknown, row: never) => ReactNode }>; request: (params: Record<string, unknown>) => Promise<{ data: Array<Record<string, unknown>> }> }) => {
      const [rows, setRows] = React.useState<Array<Record<string, unknown>>>([])
      React.useEffect(() => {
        void request({}).then((result) => setRows(result.data))
      }, [request])
      const statusColumn = columns.find((column) => column.dataIndex === 'status')

      return React.createElement(
        'div',
        null,
        React.createElement(
          'select',
          { 'aria-label': '状态筛选' },
          Object.entries(statusColumn?.valueEnum ?? {}).map(([value, option]) => React.createElement('option', { key: value, value }, option.text))
        ),
        rows.map((row) =>
          React.createElement(
            'section',
            { key: String(row.id) },
            columns.map((column) =>
              React.createElement(
                'div',
                { key: column.key ?? column.dataIndex ?? column.title },
                column.render ? column.render(row[column.dataIndex ?? ''], row as never) : String(row[column.dataIndex ?? ''] ?? '')
              )
            )
          )
        )
      )
    }
  }
})

vi.mock('./components/StockAdjustmentModal', () => ({ StockAdjustmentModal: () => null }))

function renderWithQuery(ui: ReactNode, initialEntries?: string[]) {
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter initialEntries={initialEntries} future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
        {ui}
      </MemoryRouter>
    </QueryClientProvider>
  )
}

const soldProduct = {
  id: 7,
  title: '售罄商品',
  description: '库存已清零',
  status: 'SOLD' as const,
  category_id: 1,
  price_cent: 1200,
  condition_level: 'GOOD',
  stock: 0,
  images: [],
  cover_url: '/uploads/product_image/first.jpg',
  updated_at: '2026-08-10T00:00:00Z'
}

describe('商家商品售罄页面', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.stubGlobal(
      'getComputedStyle',
      () =>
        ({
          width: '0px',
          height: '0px',
          scrollbarColor: '',
          scrollbarWidth: '',
          getPropertyValue: () => ''
        }) as unknown as CSSStyleDeclaration
    )
    vi.mocked(api.categories).mockResolvedValue({ data: { data: { items: [] } } } as never)
  })

  it('在商品列表把 SOLD 渲染为售罄并提供补库存入口，不显示已关闭入口或筛选项', async () => {
    vi.mocked(api.products).mockResolvedValue({
      data: { data: { items: [soldProduct], total: 1, page: 1, page_size: 20 } }
    } as never)

    renderWithQuery(<ListPage />)

    expect(await screen.findByText('售罄商品')).toBeInTheDocument()
    expect(screen.getAllByText('售罄').length).toBeGreaterThan(0)
    expect(screen.getAllByRole('button', { name: '调整库存' }).length).toBeGreaterThan(0)
    expect(screen.queryByRole('button', { name: '关闭' })).not.toBeInTheDocument()
    expect(screen.queryByText('已关闭')).not.toBeInTheDocument()
  })

  it('在商品列表默认展示第一张上传图片，点击查看更多后才请求详情图片', async () => {
    vi.mocked(api.products).mockResolvedValue({
      data: { data: { items: [soldProduct], total: 1, page: 1, page_size: 20 } }
    } as never)
    vi.mocked(api.productDetail).mockResolvedValue({
      data: {
        data: {
          product: {
            ...soldProduct,
            images: [11, 12],
            image_urls: ['/uploads/product_image/first.jpg', '/uploads/product_image/second.jpg']
          }
        }
      }
    } as never)

    renderWithQuery(<ListPage />)

    const cover = await screen.findByAltText('售罄商品首图')
    expect(cover).toHaveAttribute('src', 'http://localhost:8080/uploads/product_image/first.jpg')
    expect(api.productDetail).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: '查看更多' }))

    await waitFor(() => {
      expect(api.productDetail).toHaveBeenCalledWith(7)
    })
  })

  it('在商品详情把 SOLD 渲染为售罄并保留补库存入口，不显示关闭商品操作', async () => {
    vi.mocked(api.productDetail).mockResolvedValue({ data: { data: { product: soldProduct } } } as never)

    renderWithQuery(
      <Routes>
        <Route path="/merchant/products/:productId" element={<DetailPage />} />
      </Routes>,
      ['/merchant/products/7']
    )

    expect(await screen.findByText('售罄')).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: '调整库存' }).length).toBeGreaterThan(0)
    expect(screen.queryByRole('button', { name: '关闭商品' })).not.toBeInTheDocument()
  })
})
