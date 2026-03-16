import { PageContainer, ProTable, type ProColumns } from '@ant-design/pro-components'
import { Tag, message } from 'antd'
import { getCommonStatusText } from '@/constants/status'
import { api } from '@/services/api'

type AdminLogItem = {
  id: number
  request_id: string
  operator_type: string
  operator_id: number
  action: string
  resource_type: string
  resource_id: number
  from_status?: string | null
  to_status?: string | null
  result_code: number
  created_at: string
}

type AdminLogResp = {
  items: AdminLogItem[]
  total: number
  page: number
  page_size: number
}

export function ListPage() {
  const columns: ProColumns<AdminLogItem>[] = [
    {
      title: '时间',
      dataIndex: 'created_at',
      valueType: 'dateTime',
      search: false,
      width: 180
    },
    {
      title: '操作者类型',
      dataIndex: 'operator_type',
      valueType: 'select',
      valueEnum: {
        ADMIN: { text: 'ADMIN' },
        MERCHANT: { text: 'MERCHANT' },
        BUYER: { text: 'BUYER' }
      },
      width: 120
    },
    {
      title: '操作者ID',
      dataIndex: 'operator_id',
      search: false,
      width: 100
    },
    {
      title: '动作',
      dataIndex: 'action',
      width: 180
    },
    {
      title: '资源类型',
      dataIndex: 'resource_type',
      width: 120
    },
    {
      title: '资源ID',
      dataIndex: 'resource_id',
      search: false,
      width: 100
    },
    {
      title: '状态流转',
      key: 'status_flow',
      search: false,
      render: (_, row) => `${getCommonStatusText(row.from_status)} -> ${getCommonStatusText(row.to_status)}`
    },
    {
      title: '结果码',
      dataIndex: 'result_code',
      search: false,
      width: 100,
      render: (_, row) => (row.result_code === 0 ? <Tag color="success">0</Tag> : <Tag color="error">{row.result_code}</Tag>)
    },
    {
      title: '请求ID',
      dataIndex: 'request_id',
      search: false,
      copyable: true,
      ellipsis: true,
      width: 220
    }
  ]

  return (
    <PageContainer title="全局操作日志">
      <ProTable<AdminLogItem>
        rowKey="id"
        columns={columns}
        pagination={{ pageSize: 20 }}
        request={async (params) => {
          try {
            const query: Record<string, string | number> = {
              page: params.current ?? 1,
              page_size: params.pageSize ?? 20
            }
            if (params.operator_type) query.operator_type = params.operator_type as string
            if (params.action) query.action = String(params.action).trim()
            if (params.resource_type) query.resource_type = String(params.resource_type).trim()
            const res = await api.adminLogs(query)
            const payload = res.data.data as AdminLogResp
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
