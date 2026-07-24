import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { PageContainer, ProDescriptions, ProTable, type ProColumns } from '@ant-design/pro-components'
import { Alert, Button, Tag, message } from 'antd'
import { Link, useParams } from 'react-router-dom'
import { getCommonStatusText, getStatusColor, getStatusText, ORDER_STATUS_META, PRODUCT_STATUS_META, type OrderStatus } from '@/constants/status'
import { api } from '@/services/api'
import { centToYuanText } from '@/utils/price'

type OrderDetail = {
  id: number
  order_no: string
  status: OrderStatus
  quantity: number
  deal_price_cent: number
  total_deal_price_cent: number
  product: {
    id: number
    title: string
    status: string
    stock: number
    reserved_stock: number
    available_stock: number
  }
}

type OrderEventRow = {
  id: number
  event_type: string
  from_status?: string
  to_status?: string
  note?: string
}

type OrderDetailResp = {
  order_detail: OrderDetail
  events: Array<Record<string, unknown>>
}

function pick<T = unknown>(obj: Record<string, unknown>, ...keys: string[]): T | undefined {
  for (const key of keys) {
    if (key in obj) return obj[key] as T
  }
  return undefined
}

function normalizeEvent(raw: Record<string, unknown>): OrderEventRow {
  return {
    id: Number(pick(raw, 'id', 'ID') ?? 0),
    event_type: String(pick(raw, 'event_type', 'EventType') ?? ''),
    from_status: pick<string>(raw, 'from_status', 'FromStatus'),
    to_status: pick<string>(raw, 'to_status', 'ToStatus'),
    note: pick<string>(raw, 'note', 'Note')
  }
}

export function DetailPage() {
  const { orderId = '' } = useParams()
  const queryClient = useQueryClient()
  const detail = useQuery({
    queryKey: ['order-detail', orderId],
    queryFn: async () => (await api.orderDetail(orderId)).data.data as OrderDetailResp
  })

  const completeMutation = useMutation({
    mutationFn: async () => api.orderComplete(orderId, '完成交易'),
    onSuccess: async () => {
      message.success('订单已完成')
      await queryClient.invalidateQueries({ queryKey: ['order-detail', orderId] })
      await queryClient.invalidateQueries({ queryKey: ['merchant-orders'] })
      await queryClient.invalidateQueries({ queryKey: ['merchant-products'] })
    },
    onError: (err) => {
      message.error((err as Error).message)
    }
  })

  const closeMutation = useMutation({
    mutationFn: async () => api.orderClose(orderId, '关闭订单'),
    onSuccess: async () => {
      message.success('订单已关闭')
      await queryClient.invalidateQueries({ queryKey: ['order-detail', orderId] })
      await queryClient.invalidateQueries({ queryKey: ['merchant-orders'] })
      await queryClient.invalidateQueries({ queryKey: ['merchant-products'] })
    },
    onError: (err) => {
      message.error((err as Error).message)
    }
  })

  if (detail.isLoading) return <p>加载中...</p>
  if (detail.error) return <p className="error">{(detail.error as Error).message}</p>
  if (!detail.data) return <p>暂无数据</p>

  const data = detail.data
  const order = data.order_detail
  const events = (data.events ?? []).map((item) => normalizeEvent(item))
  const eventColumns: ProColumns<OrderEventRow>[] = [
    {
      title: '事件',
      dataIndex: 'event_type'
    },
    {
      title: '状态流转',
      key: 'status_flow',
      render: (_, row) => `${getCommonStatusText(row.from_status)} -> ${getCommonStatusText(row.to_status)}`
    },
    {
      title: '备注',
      dataIndex: 'note'
    }
  ]

  return (
    <PageContainer
      title="订单详情"
      subTitle={order.order_no}
      extra={
        order.status === 'CREATED'
          ? [
              <Button key="complete" type="primary" loading={completeMutation.isPending} onClick={() => completeMutation.mutate()}>
                完成订单
              </Button>,
              <Button key="close" danger loading={closeMutation.isPending} onClick={() => closeMutation.mutate()}>
                关闭订单
              </Button>
            ]
          : undefined
      }
    >
      {(completeMutation.error || closeMutation.error) ? <Alert type="error" showIcon message={((completeMutation.error ?? closeMutation.error) as Error).message} style={{ marginBottom: 16 }} /> : null}

      <ProDescriptions<OrderDetail>
        column={2}
        columns={[
          { title: '订单号', dataIndex: 'order_no' },
          {
            title: '状态',
            dataIndex: 'status',
            render: (_, row) => <Tag color={getStatusColor(ORDER_STATUS_META, row.status)}>{getStatusText(ORDER_STATUS_META, row.status)}</Tag>
          },
          { title: '数量', dataIndex: 'quantity' },
          { title: '单件成交价(元)', dataIndex: 'deal_price_cent', render: (_, row) => centToYuanText(row.deal_price_cent) },
          { title: '订单总价(元)', dataIndex: 'total_deal_price_cent', render: (_, row) => centToYuanText(row.total_deal_price_cent) },
          {
            title: '商品',
            key: 'product',
            render: (_, row) => (
              <span><Link to={`/merchant/products/${row.product.id}`}>{row.product.title}</Link> ({getStatusText(PRODUCT_STATUS_META, row.product.status)})</span>
            )
          },
          { title: '商品总库存', key: 'stock', render: (_, row) => row.product.stock },
          { title: '商品已预占', key: 'reserved_stock', render: (_, row) => row.product.reserved_stock },
          { title: '商品可售库存', key: 'available_stock', render: (_, row) => row.product.available_stock }
        ]}
        dataSource={order}
      />

      <ProTable<OrderEventRow> rowKey="id" style={{ marginTop: 16 }} search={false} options={false} pagination={false} columns={eventColumns} dataSource={events} />
    </PageContainer>
  )
}
