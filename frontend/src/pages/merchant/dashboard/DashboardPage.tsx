import { useQuery } from '@tanstack/react-query'
import { PageContainer, ProCard } from '@ant-design/pro-components'
import { Statistic, Table } from 'antd'
import { ORDER_STATUS_META, PRODUCT_STATUS_META, getStatusText } from '@/constants/status'
import { http } from '@/services/http'
import { centToYuanNumber } from '@/utils/price'

type StatMap = Record<string, number>

type MerchantDashboard = {
  product_stats: StatMap
  order_stats: StatMap
  on_shelf_total_amount_cent?: number
}

type StatusRow = {
  key: string
  status: string
  count: number
  color: string
}

const PRODUCT_STATUS_ORDER = ['draft', 'on_shelf', 'locked', 'off_shelf', 'sold']
const ORDER_STATUS_ORDER = ['created', 'completed', 'closed']
const PRODUCT_STATUS_COLORS: Record<string, string> = {
  draft: '#9ca3af',
  on_shelf: '#22c55e',
  locked: '#f59e0b',
  off_shelf: '#3b82f6',
  sold: '#2563eb'
}
const ORDER_STATUS_COLORS: Record<string, string> = {
  created: '#3b82f6',
  completed: '#22c55e',
  closed: '#6b7280'
}

function sumStats(stats: StatMap) {
  return Object.values(stats).reduce((sum, n) => sum + (Number.isFinite(n) ? n : 0), 0)
}

function toRows(
  stats: StatMap,
  order: string[],
  labels: Record<string, { text: string }>,
  colors: Record<string, string>
): StatusRow[] {
  return order.map((key) => ({
    key,
    status: getStatusText(labels, key.toUpperCase(), key),
    count: stats[key] ?? 0,
    color: colors[key] ?? '#9ca3af'
  }))
}

function buildPieGradient(rows: StatusRow[]) {
  const total = rows.reduce((sum, item) => sum + item.count, 0)
  if (total <= 0) return '#f3f4f6'
  let start = 0
  const stops: string[] = []
  rows
    .filter((item) => item.count > 0)
    .forEach((item) => {
      const end = start + (item.count / total) * 100
      stops.push(`${item.color} ${start.toFixed(2)}% ${end.toFixed(2)}%`)
      start = end
    })
  return `conic-gradient(${stops.join(', ')})`
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

export function DashboardPage() {
  const { data, isLoading, error } = useQuery({
    queryKey: ['merchant-dashboard'],
    queryFn: async () => (await http.get('/merchant/dashboard')).data.data as MerchantDashboard
  })

  if (isLoading) return <p>加载中...</p>
  if (error) return <p className="error">{(error as Error).message}</p>

  const productStats = data?.product_stats ?? {}
  const orderStats = data?.order_stats ?? {}
  const productRows = toRows(productStats, PRODUCT_STATUS_ORDER, PRODUCT_STATUS_META, PRODUCT_STATUS_COLORS)
  const orderRows = toRows(orderStats, ORDER_STATUS_ORDER, ORDER_STATUS_META, ORDER_STATUS_COLORS)
  const hasProductStats = sumStats(productStats) > 0

  return (
    <PageContainer title="全局" subTitle="当前商家经营数据总览">
      <ProCard gutter={[16, 16]} wrap>
        <ProCard colSpan={{ xs: 24, md: 6 }} bordered>
          <Statistic title="商品总数" value={sumStats(productStats)} />
        </ProCard>
        <ProCard colSpan={{ xs: 24, md: 6 }} bordered>
          <Statistic title="在售商品" value={productStats.on_shelf ?? 0} />
        </ProCard>
        <ProCard colSpan={{ xs: 24, md: 6 }} bordered>
          <Statistic title="待处理订单" value={orderStats.created ?? 0} />
        </ProCard>
        <ProCard colSpan={{ xs: 24, md: 6 }} bordered>
          <Statistic title="在售总金额" prefix="¥" value={centToYuanNumber(data?.on_shelf_total_amount_cent ?? 0)} precision={2} />
        </ProCard>
      </ProCard>

      <ProCard gutter={[16, 16]} wrap style={{ marginTop: 16 }}>
        <ProCard title="商品状态" colSpan={{ xs: 24, lg: 12 }} bordered>
          {hasProductStats ? (
            <div style={{ display: 'flex', alignItems: 'center', gap: 24, flexWrap: 'wrap', marginBottom: 16 }}>
              <div
                style={{
                  width: 170,
                  height: 170,
                  borderRadius: '50%',
                  background: buildPieGradient(productRows),
                  boxShadow: 'inset 0 0 0 1px rgba(0,0,0,0.06)'
                }}
              />
              <div style={{ minWidth: 180 }}>
                {productRows
                  .filter((item) => item.count > 0)
                  .map((item) => (
                    <div key={`legend-${item.key}`} style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, marginBottom: 8 }}>
                      <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
                        <span
                          style={{
                            width: 10,
                            height: 10,
                            borderRadius: '50%',
                            background: item.color,
                            display: 'inline-block'
                          }}
                        />
                        {item.status}
                      </span>
                      <strong>{item.count}</strong>
                    </div>
                  ))}
              </div>
            </div>
          ) : (
            <p className="muted">暂无商品状态数据</p>
          )}
          <Table<StatusRow> columns={columns} dataSource={productRows} pagination={false} size="small" />
        </ProCard>
        <ProCard title="订单状态" colSpan={{ xs: 24, lg: 12 }} bordered>
          <Table<StatusRow> columns={columns} dataSource={orderRows} pagination={false} size="small" />
        </ProCard>
      </ProCard>
    </PageContainer>
  )
}
