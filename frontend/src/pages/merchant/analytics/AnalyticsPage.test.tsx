import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { http } from '@/services/http'
import AnalyticsPage from './AnalyticsPage'

vi.mock('@/services/http', () => ({ http: { get: vi.fn() } }))
vi.mock('@ant-design/pro-components', () => ({
  PageContainer: ({ title, children }: { title: string; children: ReactNode }) => <main><h1>{title}</h1>{children}</main>,
  ProCard: ({ title, children }: { title?: string; children: ReactNode }) => <section>{title && <h2>{title}</h2>}{children}</section>
}))

const totals = { page_views: 147, devices: 32, product_clicks: 81, favorite_clicks: 13, contact_clicks: 20, contact_confirms: 16 }
const report = {
  summary: totals,
  daily: [{ date: '2026-11-28', ...totals }],
  pages: [{ page: 'pages/product/detail/index', page_views: 38, devices: 9 }],
  actions: [{ name: 'contact_confirm', page: 'pages/product/detail/index', source: 'detail_want', product_id: 215, clicks: 7 }]
}

function renderReport() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}><AnalyticsPage /></QueryClientProvider>)
}

describe('商家小程序统计', () => {
  beforeEach(() => {
    vi.spyOn(Date, 'now').mockReturnValue(Date.parse('2026-11-27T16:10:00Z'))
    vi.mocked(http.get).mockReset().mockResolvedValue({ data: { data: report } })
  })
  afterEach(() => vi.mocked(Date.now).mockRestore())

  it('按北京时间初始化近七天，展示汇总、每日、页面、商品和按钮入口', async () => {
    renderReport()
    expect(await screen.findByText('详情页「我想要」')).toBeInTheDocument()
    expect(http.get).toHaveBeenCalledWith('/merchant/analytics', expect.objectContaining({
      params: { start_date: '2026-11-22', end_date: '2026-11-28' }
    }))
    expect(screen.getByText('去重访问设备', { selector: '.ant-statistic-title' }).parentElement).toHaveTextContent('32')
    expect(screen.getByText('2026-11-28').closest('tr')).toHaveTextContent('147')
    expect(screen.getByText('215').closest('tr')).toHaveTextContent('确认拨号商品详情详情页「我想要」2157')
  })

  it('微信、抖音与日期筛选发送各自参数，过滤后展示新结果', async () => {
    renderReport()
    await screen.findByText('215')
    vi.mocked(http.get).mockResolvedValue({ data: { data: { ...report, pages: [], actions: [] } } })
    fireEvent.click(screen.getByRole('radio', { name: '微信' }))
    await screen.findByText('所选日期和平台暂无按钮点击')
    expect(http.get).toHaveBeenLastCalledWith('/merchant/analytics', expect.objectContaining({
      params: { start_date: '2026-11-22', end_date: '2026-11-28', platform: 'weapp' }
    }))
    fireEvent.change(screen.getByLabelText('开始日期'), { target: { value: '2026-11-01' } })
    fireEvent.click(screen.getByRole('radio', { name: '抖音' }))
    await waitFor(() => expect(http.get).toHaveBeenLastCalledWith('/merchant/analytics', expect.objectContaining({
      params: { start_date: '2026-11-01', end_date: '2026-11-28', platform: 'tt' }
    })))
    expect(screen.queryByText('215')).not.toBeInTheDocument()
  })

  it('日期倒置或超过 93 天时提示修正，不发请求', async () => {
    renderReport()
    await screen.findByText('215')
    fireEvent.change(screen.getByLabelText('结束日期'), { target: { value: '2026-11-21' } })
    expect(screen.getByText('开始日期应早于或等于结束日期')).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('开始日期'), { target: { value: '2026-01-01' } })
    expect(screen.getByText('单次最多查询 93 天')).toBeInTheDocument()
    expect(http.get).toHaveBeenCalledTimes(1)
  })

  it('查询失败后允许重试', async () => {
    vi.mocked(http.get).mockRejectedValueOnce(new Error('服务暂时不可用'))
    renderReport()
    expect(await screen.findByText('服务暂时不可用')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /重\s*试/ }))
    expect(await screen.findByText('215')).toBeInTheDocument()
  })
})
