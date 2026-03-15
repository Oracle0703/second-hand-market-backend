import { PageContainer, ProTable, type ProColumns } from '@ant-design/pro-components'
import { Button, Tag, message } from 'antd'
import { useNavigate } from 'react-router-dom'
import { getStatusColor, getStatusText, ORDER_STATUS_META, toValueEnum, type OrderStatus } from '../constants/status'
import { api } from '../services/api'

type OrderItem = {
  id: number
  order_no: string
  product_id: number
  status: OrderStatus
  deal_price_cent: number
  created_at: string
}

type OrderListResp = {
  items: OrderItem[]
  total: number
  page: number
  page_size: number
}

export function MerchantOrdersPage() {
  const navigate = useNavigate()

  const columns: ProColumns<OrderItem>[] = [
    {
      title: 'ID',
      dataIndex: 'id',
      width: 80,
      search: false
    },
    {
      title: '订单号',
      dataIndex: 'order_no',
      ellipsis: true,
      formItemProps: {
        label: '关键词'
      }
    },
    {
      title: '商品ID',
      dataIndex: 'product_id',
      search: false,
      width: 100
    },
    {
      title: '状态',
      dataIndex: 'status',
      valueType: 'select',
      valueEnum: toValueEnum(ORDER_STATUS_META),
      render: (_, row) => <Tag color={getStatusColor(ORDER_STATUS_META, row.status)}>{getStatusText(ORDER_STATUS_META, row.status)}</Tag>,
      width: 120
    },
    {
      title: '成交价(分)',
      dataIndex: 'deal_price_cent',
      search: false,
      width: 120
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      valueType: 'dateTime',
      search: false,
      width: 180
    },
    {
      title: '操作',
      key: 'actions',
      search: false,
      width: 100,
      render: (_, row) => (
        <Button type="link" onClick={() => navigate(`/merchant/orders/${row.id}`)}>
          详情
        </Button>
      )
    }
  ]

  return (
    <PageContainer title="订单列表">
      <ProTable<OrderItem>
        rowKey="id"
        columns={columns}
        pagination={{ pageSize: 20 }}
        request={async (params) => {
          try {
            const query: Record<string, string | number> = {
              page: params.current ?? 1,
              page_size: params.pageSize ?? 20
            }
            if (params.order_no) query.keyword = params.order_no
            if (params.status) query.status = params.status as string
            const res = await api.orders(query)
            const payload = res.data.data as OrderListResp
            return {
              data: payload.items,
              total: payload.total,
              success: true
            }
          } catch (err) {
            message.error((err as Error).message)
            return {
              data: [],
              total: 0,
              success: false
            }
          }
        }}
      />
    </PageContainer>
  )
}
