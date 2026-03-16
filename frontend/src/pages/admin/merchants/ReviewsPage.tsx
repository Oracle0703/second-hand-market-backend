import { PageContainer, ProTable, type ProColumns } from '@ant-design/pro-components'
import { Button, Tag, message } from 'antd'
import { useNavigate } from 'react-router-dom'
import { getStatusColor, getStatusText, MERCHANT_REVIEW_STATUS_META, toValueEnum, type MerchantReviewStatus } from '@/constants/status'
import { api } from '@/services/api'

type AdminMerchantItem = {
  id: number
  merchant_no: string
  merchant_name: string
  contact_name: string
  contact_phone: string
  review_status: MerchantReviewStatus
  created_at: string
}

type AdminMerchantListResp = {
  items: AdminMerchantItem[]
  total: number
  page: number
  page_size: number
}

export function ReviewsPage() {
  const navigate = useNavigate()
  const columns: ProColumns<AdminMerchantItem>[] = [
    {
      title: 'ID',
      dataIndex: 'id',
      width: 80,
      search: false
    },
    {
      title: '商家编号',
      dataIndex: 'merchant_no',
      width: 180,
      search: false
    },
    {
      title: '商家名称',
      dataIndex: 'merchant_name'
    },
    {
      title: '联系人',
      dataIndex: 'contact_name',
      search: false,
      width: 120
    },
    {
      title: '联系电话',
      dataIndex: 'contact_phone',
      search: false,
      width: 140
    },
    {
      title: '审核状态',
      dataIndex: 'review_status',
      valueType: 'select',
      valueEnum: toValueEnum(MERCHANT_REVIEW_STATUS_META),
      render: (_, row) => <Tag color={getStatusColor(MERCHANT_REVIEW_STATUS_META, row.review_status)}>{getStatusText(MERCHANT_REVIEW_STATUS_META, row.review_status)}</Tag>,
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
        <Button type="link" onClick={() => navigate(`/admin/merchants/reviews/${row.id}`)}>
          详情
        </Button>
      )
    }
  ]

  return (
    <PageContainer title="商家审核列表">
      <ProTable<AdminMerchantItem>
        rowKey="id"
        columns={columns}
        pagination={{ pageSize: 20 }}
        request={async (params) => {
          try {
            const query: Record<string, string | number> = {
              page: params.current ?? 1,
              page_size: params.pageSize ?? 20
            }
            if (params.review_status) query.status = params.review_status as string
            if (params.keyword) query.keyword = String(params.keyword).trim()
            const res = await api.adminMerchantReviews(query)
            const payload = res.data.data as AdminMerchantListResp
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
