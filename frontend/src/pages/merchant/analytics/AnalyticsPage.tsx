import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { PageContainer, ProCard } from '@ant-design/pro-components'
import { Alert, Button, Col, Input, Radio, Row, Space, Statistic, Table } from 'antd'
import { http, type APIResponse } from '@/services/http'
import { lastSevenDays, rangeError } from './analyticsDates'

const metrics = [
  { key: 'page_views', title: '页面曝光' },
  { key: 'devices', title: '去重访问设备' },
  { key: 'product_clicks', title: '商品点击' },
  { key: 'favorite_clicks', title: '收藏按钮点击' },
  { key: 'contact_clicks', title: '联系商家点击' },
  { key: 'contact_confirms', title: '确认拨号' }
] as const

type Counts = Record<typeof metrics[number]['key'], number>
type AnalyticsReport = {
  summary: Counts
  daily: Array<{ date: string } & Counts>
  pages: Array<{ page: string; page_views: number; devices: number }>
  actions: Array<{ name: string; page: string; source: string; product_id: number | null; clicks: number }>
}
type Platform = 'all' | 'weapp' | 'tt'

const pages: Record<string, string> = {
  'pages/home/index': '首页',
  'pages/category/index': '分类',
  'pages/search/index': '搜索',
  'pages/product/list/index': '商品列表',
  'pages/product/detail/index': '商品详情',
  'pages/favorite/index': '我的收藏',
  'pages/history/index': '浏览记录',
  'pages/login/index': '登录',
  'pages/store-guide/index': '到店导航',
  'pages/me/index': '我的'
}

const actions: Record<string, string> = {
  product_click: '查看商品',
  favorite_click: '收藏按钮点击',
  contact_click: '联系商家',
  contact_confirm: '确认拨号'
}

const sources: Record<string, string> = {
  home_carousel: '首页轮播',
  home_grid: '首页商品列表',
  category_list: '分类商品',
  search_results: '搜索结果',
  product_list: '商品列表',
  favorite_list: '收藏列表',
  history_list: '浏览记录',
  detail_favorite: '详情页收藏',
  detail_unfavorite: '详情页取消收藏',
  favorite_unfavorite: '收藏页取消收藏',
  home_store_phone: '首页门店电话',
  home_want: '首页「我想要」',
  category_want: '分类页「我想要」',
  favorite_want: '收藏页「我想要」',
  detail_want: '详情页「我想要」'
}

const pageColumn = { title: '页面', dataIndex: 'page', render: (page: string) => pages[page] ?? page }
const dailyColumns = [
  { title: '日期', dataIndex: 'date' },
  ...metrics.map(({ key, title }) => ({ title, dataIndex: key, align: 'right' as const }))
]
const pageColumns = [
  pageColumn,
  { title: '页面曝光', dataIndex: 'page_views', align: 'right' as const },
  { title: '去重访问设备', dataIndex: 'devices', align: 'right' as const }
]
const actionColumns = [
  { title: '动作', dataIndex: 'name', render: (name: string) => actions[name] ?? name },
  pageColumn,
  { title: '入口', dataIndex: 'source', render: (source: string) => sources[source] ?? source },
  { title: '商品 ID', dataIndex: 'product_id', render: (id: number | null) => id || '—' },
  { title: '次数', dataIndex: 'clicks', align: 'right' as const }
]

export default function AnalyticsPage() {
  const [range, setRange] = useState(lastSevenDays)
  const [platform, setPlatform] = useState<Platform>('all')
  const invalidRange = rangeError(range.start, range.end)
  const report = useQuery({
    queryKey: ['merchant-analytics', range.start, range.end, platform],
    enabled: !invalidRange,
    queryFn: async ({ signal }) => {
      const params = { start_date: range.start, end_date: range.end, ...(platform === 'all' ? {} : { platform }) }
      const response = await http.get<APIResponse<AnalyticsReport>>('/merchant/analytics', { params, signal })
      return response.data.data
    }
  })

  return (
    <PageContainer title="小程序统计">
      <Space direction="vertical" size="middle" style={{ display: 'flex' }}>
        <ProCard bordered>
          <Space wrap size="middle">
            <Space>
              <label htmlFor="analytics-start">开始日期</label>
              <Input id="analytics-start" type="date" value={range.start}
                onChange={(event) => setRange({ ...range, start: event.target.value })} />
            </Space>
            <Space>
              <label htmlFor="analytics-end">结束日期</label>
              <Input id="analytics-end" type="date" value={range.end}
                onChange={(event) => setRange({ ...range, end: event.target.value })} />
            </Space>
            <span className="muted">北京时间</span>
            <Radio.Group aria-label="小程序平台" value={platform} onChange={(event) => setPlatform(event.target.value)}>
              <Radio.Button value="all">全部平台</Radio.Button>
              <Radio.Button value="weapp">微信</Radio.Button>
              <Radio.Button value="tt">抖音</Radio.Button>
            </Radio.Group>
          </Space>
        </ProCard>
        {invalidRange && <Alert type="warning" showIcon message={invalidRange} />}
        {report.isLoading && <p role="status">正在加载统计数据…</p>}
        {report.isError && (
          <Alert type="error" showIcon message="统计数据加载失败" description={report.error.message}
            action={<Button onClick={() => void report.refetch()}>重试</Button>} />
        )}
        {report.data && !invalidRange && (
          <>
            <ProCard bordered>
              <Row gutter={[16, 16]}>
                {metrics.map(({ key, title }) => (
                  <Col key={key} xs={12} md={8} xl={4}>
                    <Statistic title={title} value={report.data.summary[key]} />
                  </Col>
                ))}
              </Row>
            </ProCard>
            <ProCard title="每日统计" bordered>
              <Table rowKey="date" columns={dailyColumns} dataSource={report.data.daily} size="small"
                scroll={{ x: 700 }} pagination={{ pageSize: 10, hideOnSinglePage: true, showSizeChanger: false }}
                locale={{ emptyText: '所选日期和平台暂无访问或点击' }} />
            </ProCard>
            <ProCard title="页面曝光明细" bordered>
              <Table rowKey="page" columns={pageColumns} dataSource={report.data.pages} size="small"
                pagination={false} locale={{ emptyText: '所选日期和平台暂无页面曝光' }} />
            </ProCard>
            <ProCard title="按钮点击明细（前 100 项）" bordered>
              <Table rowKey={(row) => [row.name, row.page, row.source, row.product_id].join('|')}
                columns={actionColumns} dataSource={report.data.actions} size="small" scroll={{ x: 520 }}
                pagination={{ pageSize: 10, hideOnSinglePage: true, showSizeChanger: false }}
                locale={{ emptyText: '所选日期和平台暂无按钮点击' }} />
            </ProCard>
          </>
        )}
      </Space>
    </PageContainer>
  )
}
