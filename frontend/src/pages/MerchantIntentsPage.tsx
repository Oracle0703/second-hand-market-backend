import { PageContainer, ProTable, type ProColumns } from '@ant-design/pro-components'
import { Button, Tag, message } from 'antd'
import { useNavigate } from 'react-router-dom'
import { getStatusColor, getStatusText, INTENT_STATUS_META, toValueEnum, type IntentStatus } from '../constants/status'
import { api } from '../services/api'

type IntentItem = {
  id: number
  intent_no: string
  product_id: number
  product_title: string
  status: IntentStatus
  contact_name?: string | null
  contact_phone?: string | null
  contact_wechat?: string | null
  created_at: string
  updated_at: string
}

type IntentListResp = {
  items: IntentItem[]
  total: number
  page: number
  page_size: number
}

export function MerchantIntentsPage() {
  const navigate = useNavigate()
  const columns: ProColumns<IntentItem>[] = [
    {
      title: 'ID',
      dataIndex: 'id',
      width: 80,
      search: false
    },
    {
      title: '意向单号',
      dataIndex: 'intent_no',
      ellipsis: true,
      search: false
    },
    {
      title: '商品',
      dataIndex: 'product_title',
      ellipsis: true,
      search: false
    },
    {
      title: '状态',
      dataIndex: 'status',
      valueType: 'select',
      valueEnum: toValueEnum(INTENT_STATUS_META),
      render: (_, row) => <Tag color={getStatusColor(INTENT_STATUS_META, row.status)}>{getStatusText(INTENT_STATUS_META, row.status)}</Tag>,
      width: 120
    },
    {
      title: '联系方式',
      key: 'contact',
      search: false,
      render: (_, row) => {
        const parts = [row.contact_name, row.contact_phone, row.contact_wechat].filter(Boolean)
        return parts.length > 0 ? parts.join(' / ') : '-'
      }
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      valueType: 'dateTime',
      search: false,
      width: 180
    },
    {
      title: '关键词',
      dataIndex: 'keyword',
      hideInTable: true
    },
    {
      title: '操作',
      key: 'actions',
      search: false,
      width: 100,
      render: (_, row) => (
        <Button type="link" onClick={() => navigate(`/merchant/intents/${row.id}`)}>
          详情
        </Button>
      )
    }
  ]

  return (
    <PageContainer title="意向线索">
      <ProTable<IntentItem>
        rowKey="id"
        columns={columns}
        pagination={{ pageSize: 20 }}
        request={async (params) => {
          try {
            const query: Record<string, string | number> = {
              page: params.current ?? 1,
              page_size: params.pageSize ?? 20
            }
            if (params.status) query.status = params.status as string
            if (params.keyword) query.keyword = String(params.keyword).trim()
            const res = await api.merchantIntents(query)
            const payload = res.data.data as IntentListResp
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
