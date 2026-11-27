import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, useLocation } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { http } from '@/services/http'
import ExplorePage from './ExplorePage'
import { exploreParams, initialForm, readExploreLocation, type Scope } from './explore'

vi.mock('@/services/http', () => ({ http: { get: vi.fn() } }))
vi.mock('@ant-design/pro-components', () => ({
  PageContainer: ({ title, children }: { title: string; children: ReactNode }) => <main><h1>{title}</h1>{children}</main>,
  ProCard: ({ title, children }: { title?: string; children: ReactNode }) => <section>{title && <h2>{title}</h2>}{children}</section>
}))

const row = { name: 'catalog_open', event_id: 'catalog-101', platform: 'tt', page: 'pages/catalog/index', source: 'top',
  product_id: 81, merchant_id: 12, occurred_at: '2026-11-27T18:05:12Z', properties: { discounted: false, level: 3 } }
const data = { total: 41, devices: 9, groups_total: 1, groups: [{ value_type: 'string', value: 'new-season', count: 41, devices: 9 }], events: [row], page_no: 1, page_size: 20 }
const dimensions = { names: ['catalog_open'], sources: ['top'], pages: ['pages/catalog/index'], property_keys: ['level', 'discounted'] }

function CurrentSearch() {
  return <output data-testid="query-string">{useLocation().search}</output>
}
function currentQuery() {
  return new URLSearchParams(screen.getByTestId('query-string').textContent ?? '')
}
function setup(scope: Scope, query = '') {
  const route = '/' + scope + '/analytics/explore'
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}><MemoryRouter initialEntries={[route + query]}><ExplorePage scope={scope} /><CurrentSearch /></MemoryRouter></QueryClientProvider>)
}

describe('事件查询', () => {
  beforeEach(() => {
    vi.mocked(http.get).mockReset().mockImplementation(async (url: string) => ({ data: { data: url.endsWith('/dimensions') ? dimensions : data } }))
  })

  it('从动态候选选择或自由输入，组合数值和布尔条件，提交后保存到链接', async () => {
    const { container } = setup('merchant')
    await screen.findByText('new-season')
    expect(container.querySelector('datalist#explore-names option')).toHaveAttribute('value', 'catalog_open')
    const get = vi.mocked(http.get)
    const count = get.mock.calls.length
    fireEvent.change(screen.getByLabelText('事件名'), { target: { value: 'new_event' } })
    fireEvent.change(screen.getByLabelText('分组维度'), { target: { value: 'property' } })
    fireEvent.change(screen.getByLabelText('分组属性名'), { target: { value: 'new_key' } })
    fireEvent.click(screen.getByRole('button', { name: '添加条件' }))
    fireEvent.change(screen.getByLabelText('属性名 1'), { target: { value: 'level' } })
    fireEvent.change(screen.getByLabelText('类型 1'), { target: { value: 'number' } })
    fireEvent.change(screen.getByLabelText('比较方式 1'), { target: { value: 'gte' } })
    fireEvent.change(screen.getByLabelText('值 1'), { target: { value: '3.5' } })
    fireEvent.click(screen.getByRole('button', { name: '添加条件' }))
    fireEvent.change(screen.getByLabelText('属性名 2'), { target: { value: 'discounted' } })
    fireEvent.change(screen.getByLabelText('类型 2'), { target: { value: 'boolean' } })
    fireEvent.change(screen.getByLabelText('值 2'), { target: { value: 'false' } })
    expect(get.mock.calls).toHaveLength(count)
    fireEvent.submit(screen.getByRole('button', { name: /查\s*询/ }).closest('form')!)
    const filters = JSON.stringify([{ key: 'level', op: 'gte', value: 3.5 }, { key: 'discounted', op: 'eq', value: false }])
    await waitFor(() => expect(get).toHaveBeenCalledWith('/merchant/analytics/explore', expect.objectContaining({
      params: expect.objectContaining({ name: 'new_event', group_by: 'prop.new_key', filters, page_no: 1 })
    })))
    expect(currentQuery().get('filters')).toBe(filters)
    expect(get).toHaveBeenCalledWith('/merchant/analytics/dimensions', expect.objectContaining({
      params: expect.objectContaining({ name: 'new_event' })
    }))
    expect(screen.queryByLabelText('商户 ID')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: '重置筛选' }))
    expect(screen.queryByLabelText('属性名 1')).toBeNull()
    expect(currentQuery().has('filters')).toBe(false)
  })

  it('管理员按商户查询，链接还原条件，支持明细分页与查看属性', async () => {
    const { container } = setup('admin', '?start_date=2026-11-27&end_date=2026-11-28&merchant_id=12&group_by=merchant_id&platform=tt&page_no=1&page_size=20')
    await screen.findByText('new-season')
    expect(screen.getByLabelText('商户 ID')).toHaveValue('12')
    expect(vi.mocked(http.get)).toHaveBeenCalledWith('/admin/analytics/dimensions', expect.objectContaining({
      params: { start_date: '2026-11-27', end_date: '2026-11-28', merchant_id: '12', platform: 'tt' }
    }))
    expect(screen.getByText('2026-11-28 02:05:12')).toBeInTheDocument()
    fireEvent.click(container.querySelector('.ant-table-row-expand-icon')!)
    expect(screen.getByText('catalog-101')).toBeInTheDocument()
    expect(screen.getByText('discounted')).toBeInTheDocument()
    expect(screen.getByText('false')).toBeInTheDocument()
    fireEvent.click(container.querySelector('.ant-pagination-next button')!)
    await waitFor(() => expect(vi.mocked(http.get)).toHaveBeenCalledWith('/admin/analytics/explore', expect.objectContaining({
      params: expect.objectContaining({ page_no: 2, merchant_id: '12', group_by: 'merchant_id' })
    })))
    expect(currentQuery().get('page_no')).toBe('2')
  })

  it('无效链接显示错误，不运行明细查询，重置后可继续', async () => {
    setup('merchant', '?filters=broken')
    expect(screen.getByText('链接中的属性条件格式无效')).toBeInTheDocument()
    expect(vi.mocked(http.get).mock.calls.some(([url]) => url.endsWith('/explore'))).toBe(false)
    fireEvent.click(screen.getByRole('button', { name: '重置筛选' }))
    expect(await screen.findByText('new-season')).toBeInTheDocument()
  })

  it('精确保留筛选值的 JSON 类型，商户链接不接受管理员范围', () => {
    const form = initialForm()
    form.conditions = [
      { id: 1, key: 'tag', type: 'string', op: 'contains', value: '3' },
      { id: 2, key: 'rank', type: 'number', op: 'lte', value: '0' },
      { id: 3, key: 'enabled', type: 'boolean', op: 'ne', value: 'false' }
    ]
    const params = exploreParams(form, 'merchant')
    expect(JSON.parse(params.filters)).toEqual([{ key: 'tag', op: 'contains', value: '3' }, { key: 'rank', op: 'lte', value: 0 }, { key: 'enabled', op: 'ne', value: false }])
    const restored = readExploreLocation(new URLSearchParams({ ...params, merchant_id: '99' }), 'merchant')
    expect(restored.form.conditions).toEqual(form.conditions)
    expect(restored.params).not.toHaveProperty('merchant_id')
    expect(readExploreLocation(new URLSearchParams({ group_by: 'merchant_id' }), 'merchant').error).toBeTruthy()
    form.conditions[1].value = ''
    expect(() => exploreParams(form, 'merchant')).toThrow('数值')
  })
})
