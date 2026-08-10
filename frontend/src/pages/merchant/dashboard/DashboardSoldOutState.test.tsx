import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { http } from '@/services/http'
import { DashboardPage } from './DashboardPage'

vi.mock('@/services/http', () => ({
  http: {
    get: vi.fn()
  }
}))

vi.mock('@ant-design/pro-components', async () => {
  const React = await vi.importActual<typeof import('react')>('react')

  return {
    PageContainer: ({ title, children }: { title: string; children?: React.ReactNode }) => React.createElement('main', null, React.createElement('h1', null, title), children),
    ProCard: ({ title, children }: { title?: string; children?: React.ReactNode }) =>
      React.createElement('section', null, title ? React.createElement('h2', null, title) : null, children)
  }
})

vi.mock('antd', async () => {
  const actual = await vi.importActual<typeof import('antd')>('antd')
  const React = await vi.importActual<typeof import('react')>('react')

  return {
    ...actual,
    Table: ({ columns, dataSource }: { columns: Array<{ dataIndex: string; title: string }>; dataSource: Array<Record<string, unknown>> }) =>
      React.createElement(
        'table',
        null,
        React.createElement(
          'tbody',
          null,
          dataSource.map((row) =>
            React.createElement(
              'tr',
              { key: String(row.key) },
              columns.map((column) => React.createElement('td', { key: column.dataIndex }, String(row[column.dataIndex] ?? '')))
            )
          )
        )
      )
  }
})

function renderDashboard() {
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <DashboardPage />
    </QueryClientProvider>
  )
}

describe('商家 Dashboard 商品状态', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('基于五状态商品统计显示售罄和正确总数，已关闭只保留在订单统计中', async () => {
    vi.mocked(http.get).mockResolvedValue({
      data: {
        data: {
          product_stats: { draft: 1, on_shelf: 1, locked: 1, off_shelf: 1, sold: 1 },
          order_stats: { created: 1, completed: 1, closed: 1 },
          on_shelf_total_amount_cent: 1200
        }
      }
    } as never)

    renderDashboard()

    expect((await screen.findAllByText('售罄')).length).toBeGreaterThan(0)
    expect(screen.getByText('商品总数').parentElement).toHaveTextContent('5')
    expect(screen.getByText('商品状态').parentElement).not.toHaveTextContent('已关闭')
    expect(screen.getAllByText('已关闭')).toHaveLength(1)
  })
})
