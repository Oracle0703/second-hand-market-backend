import { useQuery } from '@tanstack/react-query'
import { PageContainer, ProCard } from '@ant-design/pro-components'
import { Statistic, Table } from 'antd'
import { ORDER_STATUS_META, PRODUCT_STATUS_META, getStatusText } from '../constants/status'
import { http } from '../services/http'

type StatMap = Record<string, number>

type MerchantDashboard = {
  product_stats: StatMap
  order_stats: StatMap
}

type StatusRow = {
  key: string
  status: string
  count: number
}

const PRODUCT_STATUS_ORDER = ['draft', 'on_shelf', 'locked', 'off_shelf', 'sold', 'closed']
const ORDER_STATUS_ORDER = ['created', 'completed', 'closed']

function sumStats(stats: StatMap) {
  return Object.values(stats).reduce((sum, n) => sum + (Number.isFinite(n) ? n : 0), 0)
}

function toRows(stats: StatMap, order: string[], labels: Record<string, { text: string }>): StatusRow[] {
  return order.map((key) => ({
    key,
    status: getStatusText(labels, key, key),
    count: stats[key] ?? 0
  }))
}

const columns = [
  {
    title: '状态',
    dataIndex: 'status',
    key: 'status'
  },
  {
    title: '数量',
    dataIndex: 'count',
    key: 'count',
    align: 'right' as const
  }
]

export function MerchantDashboardPage() {
  const { data, isLoading, error } = useQuery({
    queryKey: ['merchant-dashboard'],
    queryFn: async () => (await http.get('/merchant/dashboard')).data.data as MerchantDashboard
  })

  if (isLoading) return <p>加载中...</p>
  if (error) return <p className="error">{(error as Error).message}</p>

  const productStats = data?.product_stats ?? {}
  const orderStats = data?.order_stats ?? {}

  return (
    <PageContainer title="全局" subTitle="当前商家经营数据总览">
      <ProCard gutter={[16, 16]} wrap>
        <ProCard colSpan={{ xs: 24, md: 8 }} bordered>
          <Statistic title="商品总数" value={sumStats(productStats)} />
        </ProCard>
        <ProCard colSpan={{ xs: 24, md: 8 }} bordered>
          <Statistic title="在售商品" value={productStats.on_shelf ?? 0} />
        </ProCard>
        <ProCard colSpan={{ xs: 24, md: 8 }} bordered>
          <Statistic title="待处理订单" value={orderStats.created ?? 0} />
        </ProCard>
      </ProCard>

      <ProCard gutter={[16, 16]} wrap style={{ marginTop: 16 }}>
        <ProCard title="商品状态" colSpan={{ xs: 24, lg: 12 }} bordered>
          <Table<StatusRow> columns={columns} dataSource={toRows(productStats, PRODUCT_STATUS_ORDER, PRODUCT_STATUS_META)} pagination={false} size="small" />
        </ProCard>
        <ProCard title="订单状态" colSpan={{ xs: 24, lg: 12 }} bordered>
          <Table<StatusRow> columns={columns} dataSource={toRows(orderStats, ORDER_STATUS_ORDER, ORDER_STATUS_META)} pagination={false} size="small" />
        </ProCard>
      </ProCard>
    </PageContainer>
  )
}
