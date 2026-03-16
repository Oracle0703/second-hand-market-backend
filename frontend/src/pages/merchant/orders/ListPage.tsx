import { useQuery } from '@tanstack/react-query'
import { PageContainer, ProTable, type ProColumns } from '@ant-design/pro-components'
import { Button, Tag, message } from 'antd'
import { useNavigate } from 'react-router-dom'
import { getStatusColor, getStatusText, ORDER_STATUS_META, toValueEnum, type OrderStatus } from '@/constants/status'
import { api } from '@/services/api'
import { centToYuanText } from '@/utils/price'

type OrderItem = {
  id: number
  order_no: string
  product_id: number
  status: OrderStatus
  deal_price_cent: number
  created_at: string
  category_level1_id?: number | null
  category_level1_name?: string | null
  category_level2_id?: number | null
  category_level2_name?: string | null
}

type OrderListResp = {
  items: OrderItem[]
  total: number
  page: number
  page_size: number
}

type CategoryItem = {
  ID?: number
  id?: number
  Name?: string
  name?: string
}

function categoryId(item: CategoryItem) {
  return Number(item.ID ?? item.id ?? 0)
}

function categoryName(item: CategoryItem) {
  return item.Name ?? item.name ?? ''
}

export function ListPage() {
  const navigate = useNavigate()
  const level1 = useQuery({
    queryKey: ['categories', 'level1'],
    queryFn: async () => (await api.categories(1)).data.data.items as CategoryItem[]
  })

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
      title: '一级分类',
      dataIndex: 'category_level1_name',
      search: false,
      width: 120,
      render: (_, row) => row.category_level1_name || '-'
    },
    {
      title: '二级分类',
      dataIndex: 'category_level2_name',
      search: false,
      width: 120,
      render: (_, row) => row.category_level2_name || '-'
    },
    {
      title: '成交价(元)',
      dataIndex: 'deal_price_cent',
      search: false,
      width: 120,
      render: (_, row) => centToYuanText(row.deal_price_cent)
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
    },
    {
      title: '一级分类',
      dataIndex: 'category_level1_id',
      valueType: 'select',
      hideInTable: true,
      fieldProps: {
        options: (level1.data ?? []).map((item) => ({ value: categoryId(item), label: categoryName(item) })),
        loading: level1.isLoading,
        showSearch: true,
        optionFilterProp: 'label'
      }
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
            if (params.category_level1_id) query.category_level1_id = Number(params.category_level1_id)
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
