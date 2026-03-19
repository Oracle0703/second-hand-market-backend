import { useQuery } from '@tanstack/react-query'
import { PageContainer, ProTable, type ProColumns } from '@ant-design/pro-components'
import { message } from 'antd'
import { getCommonStatusText } from '@/constants/status'
import { api } from '@/services/api'

type MerchantLogItem = {
  id: number
  action: string
  resource_type: string
  resource_id: number
  from_status?: string | null
  to_status?: string | null
  result_code: number
  created_at: string
  request_id: string
  category_level1_id?: number | null
  category_level1_name?: string | null
  category_level2_id?: number | null
  category_level2_name?: string | null
}

type MerchantLogResp = {
  items: MerchantLogItem[]
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

const resourceTypeValueEnum = {
  product: { text: '商品' },
  order: { text: '订单' },
  intent: { text: '意向' },
  merchant: { text: '商家' },
  account: { text: '账号' }
} as const

const actionValueEnum: Record<string, { text: string }> = {
  product_create: { text: '创建商品' },
  product_update: { text: '编辑商品' },
  product_on_shelf: { text: '商品上架' },
  product_off_shelf: { text: '商品下架' },
  product_close: { text: '关闭商品' },
  product_delete: { text: '删除商品' },
  product_lock: { text: '商品锁定' },
  product_order_link: { text: '商品关联订单' },
  order_create: { text: '创建订单' },
  order_complete: { text: '完成订单' },
  order_close: { text: '关闭订单' },
  merchant_reapply: { text: '商家重新提交' },
  merchant_approve: { text: '商家审核通过' },
  merchant_reject: { text: '商家审核驳回' },
  merchant_intent_contacted: { text: '意向标记已联系' },
  merchant_intent_close: { text: '关闭意向' }
}

export function ListPage() {
  const level1 = useQuery({
    queryKey: ['categories', 'level1'],
    queryFn: async () => (await api.categories(1)).data.data.items as CategoryItem[]
  })
  const columns: ProColumns<MerchantLogItem>[] = [
    {
      title: '时间',
      dataIndex: 'created_at',
      valueType: 'dateTime',
      search: false,
      width: 180
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
    },
    {
      title: '动作',
      dataIndex: 'action',
      width: 200,
      valueType: 'select',
      valueEnum: actionValueEnum,
      render: (_, row) => actionValueEnum[row.action]?.text ?? row.action
    },
    {
      title: '一级分类',
      dataIndex: 'category_level1_name',
      search: false,
      width: 120,
      render: (_, row) => row.category_level1_name || '-'
    },
    {
      title: '资源类型',
      dataIndex: 'resource_type',
      width: 120,
      valueType: 'select',
      valueEnum: resourceTypeValueEnum
    },
    {
      title: '状态流转',
      key: 'status_flow',
      search: false,
      render: (_, row) => (
        <span>
          {getCommonStatusText(row.from_status)} -&gt; {getCommonStatusText(row.to_status)}
        </span>
      )
    },
    {
      title: '请求ID',
      dataIndex: 'request_id',
      search: false,
      copyable: true,
      ellipsis: true,
      width: 220
    },
    
  ]

  return (
    <PageContainer title="商家操作日志">
      <ProTable<MerchantLogItem>
        rowKey="id"
        columns={columns}
        pagination={{ pageSize: 20 }}
        request={async (params) => {
          try {
            const query: Record<string, string | number> = {
              page: params.current ?? 1,
              page_size: params.pageSize ?? 20
            }
            if (params.action) query.action = String(params.action).trim()
            if (params.resource_type) query.resource_type = String(params.resource_type).trim()
            if (params.category_level1_id) query.category_level1_id = Number(params.category_level1_id)
            const res = await api.merchantLogs(query)
            const payload = res.data.data as MerchantLogResp
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
