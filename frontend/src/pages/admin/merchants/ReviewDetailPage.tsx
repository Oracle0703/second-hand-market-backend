import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { PageContainer, ProCard, ProDescriptions, ProTable, type ProColumns } from '@ant-design/pro-components'
import { Alert, Button, Image, Input, Space, Spin, Tag, Typography, message } from 'antd'
import { isAxiosError } from 'axios'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { getCommonStatusText, getStatusColor, getStatusText, MERCHANT_REVIEW_STATUS_META, type MerchantReviewStatus } from '@/constants/status'
import { api } from '@/services/api'

type MerchantDetail = {
  id: number
  merchant_no: string
  merchant_name: string
  contact_name: string
  contact_phone: string
  license_file_id?: number | null
  review_status: MerchantReviewStatus
  reject_reason?: string | null
  reviewed_by?: number | null
  reviewed_at?: string | null
  created_at: string
  updated_at: string
}

type AuditLogItem = {
  id: number
  merchant_id: number
  action: string
  from_status: string
  to_status: string
  reason?: string | null
  operator_type: string
  operator_id: number
  created_at: string
}

type AdminMerchantDetailResp = {
  merchant_detail: MerchantDetail
  audit_logs: AuditLogItem[]
}

export function ReviewDetailPage() {
  const { merchantId = '' } = useParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [rejectReason, setRejectReason] = useState('')
  const [approveComment, setApproveComment] = useState('')
  const [licensePreviewURL, setLicensePreviewURL] = useState<string | null>(null)
  const [licenseImageDamaged, setLicenseImageDamaged] = useState(false)

  const detailQuery = useQuery({
    queryKey: ['admin-merchant-detail', merchantId],
    queryFn: async () => (await api.adminMerchantReviewDetail(merchantId)).data.data as AdminMerchantDetailResp
  })
  const licenseFileID = detailQuery.data?.merchant_detail.license_file_id
  const licenseQuery = useQuery({
    queryKey: ['admin-license-content', licenseFileID],
    enabled: Boolean(licenseFileID),
    retry: false,
    queryFn: async () => (await api.adminLicenseContent(licenseFileID!)).data
  })

  useEffect(() => {
    setLicenseImageDamaged(false)
    if (!licenseQuery.data) {
      setLicensePreviewURL(null)
      return
    }

    const nextURL = URL.createObjectURL(licenseQuery.data)
    setLicensePreviewURL(nextURL)
    return () => URL.revokeObjectURL(nextURL)
  }, [licenseFileID, licenseQuery.data])

  const approveMutation = useMutation({
    mutationFn: async () => api.adminMerchantApprove(merchantId, approveComment || undefined),
    onSuccess: async () => {
      message.success('审核已通过')
      await queryClient.invalidateQueries({ queryKey: ['admin-merchant-detail', merchantId] })
      await queryClient.invalidateQueries({ queryKey: ['admin-merchants'] })
      setApproveComment('')
    },
    onError: (err) => {
      message.error((err as Error).message)
    }
  })

  const rejectMutation = useMutation({
    mutationFn: async () => api.adminMerchantReject(merchantId, rejectReason),
    onSuccess: async () => {
      message.success('已驳回')
      await queryClient.invalidateQueries({ queryKey: ['admin-merchant-detail', merchantId] })
      await queryClient.invalidateQueries({ queryKey: ['admin-merchants'] })
      setRejectReason('')
    },
    onError: (err) => {
      message.error((err as Error).message)
    }
  })

  if (detailQuery.isLoading) return <p>加载中...</p>
  if (detailQuery.error) return <p className="error">{(detailQuery.error as Error).message}</p>
  if (!detailQuery.data) return <p>暂无数据</p>

  const detail = detailQuery.data
  const merchant = detail.merchant_detail
  const auditLogs = detail.audit_logs ?? []
  const status = merchant.review_status
  const licenseAuthFailed =
    (isAxiosError(licenseQuery.error) && [401, 403].includes(licenseQuery.error.response?.status ?? 0)) ||
    (licenseQuery.error instanceof Error && licenseQuery.error.message.includes('登录已过期'))
  const auditColumns: ProColumns<AuditLogItem>[] = [
    {
      title: '时间',
      dataIndex: 'created_at',
      valueType: 'dateTime',
      width: 180
    },
    {
      title: '动作',
      dataIndex: 'action',
      width: 140
    },
    {
      title: '状态流转',
      key: 'status_flow',
      render: (_, row) => `${getCommonStatusText(row.from_status)} -> ${getCommonStatusText(row.to_status)}`
    },
    {
      title: '原因',
      dataIndex: 'reason'
    },
    {
      title: '操作人',
      key: 'operator',
      render: (_, row) => `${row.operator_type}#${row.operator_id}`,
      width: 140
    }
  ]

  return (
    <PageContainer
      title="审核详情"
      subTitle={merchant.merchant_name}
      extra={[
        <Button key="back" onClick={() => navigate('/admin/merchants/reviews')}>
          返回列表
        </Button>
      ]}
    >
      {(approveMutation.error || rejectMutation.error) ? <Alert type="error" showIcon message={((approveMutation.error ?? rejectMutation.error) as Error).message} style={{ marginBottom: 16 }} /> : null}

      <ProDescriptions<MerchantDetail>
        column={2}
        dataSource={merchant}
        columns={[
          { title: 'ID', dataIndex: 'id' },
          { title: '商家编号', dataIndex: 'merchant_no' },
          { title: '商家名称', dataIndex: 'merchant_name' },
          { title: '联系人', dataIndex: 'contact_name' },
          { title: '联系电话', dataIndex: 'contact_phone' },
          {
            title: '审核状态',
            dataIndex: 'review_status',
            render: (_, row) => <Tag color={getStatusColor(MERCHANT_REVIEW_STATUS_META, row.review_status)}>{getStatusText(MERCHANT_REVIEW_STATUS_META, row.review_status)}</Tag>
          },
          { title: '资质文件ID', dataIndex: 'license_file_id' },
          { title: '驳回原因', dataIndex: 'reject_reason' },
          { title: '审核人', dataIndex: 'reviewed_by' },
          { title: '审核时间', dataIndex: 'reviewed_at', valueType: 'dateTime' },
          { title: '创建时间', dataIndex: 'created_at', valueType: 'dateTime' },
          { title: '更新时间', dataIndex: 'updated_at', valueType: 'dateTime' }
        ]}
      />

      <ProCard title="营业执照" style={{ marginTop: 16 }}>
        <div
          data-license-preview
          style={{ minHeight: 280, display: 'flex', alignItems: 'center', justifyContent: 'center' }}
        >
          {!licenseFileID ? (
            <Typography.Text type="secondary">暂无营业执照</Typography.Text>
          ) : licenseQuery.isLoading ? (
            <Spin aria-label="营业执照加载中" />
          ) : licenseAuthFailed ? (
            <Typography.Text type="danger">营业执照鉴权失败</Typography.Text>
          ) : licenseQuery.error || licenseImageDamaged || !licensePreviewURL ? (
            <Typography.Text type="danger">营业执照不可用</Typography.Text>
          ) : (
            <Space direction="vertical" align="center">
              <Image
                alt="营业执照"
                src={licensePreviewURL}
                style={{ maxHeight: 520, objectFit: 'contain' }}
                onError={() => setLicenseImageDamaged(true)}
              />
              <Typography.Text type="secondary">文件 ID：{licenseFileID}</Typography.Text>
            </Space>
          )}
        </div>
      </ProCard>

      {status === 'PENDING' ? (
        <ProCard title="审核动作" style={{ marginTop: 16 }}>
          <Space direction="vertical" size={16} style={{ width: '100%' }}>
            <div>
              <div style={{ marginBottom: 8 }}>通过备注（可选）</div>
              <Input
                value={approveComment}
                onChange={(e) => setApproveComment(e.target.value)}
                placeholder="可填写审核备注"
                style={{ maxWidth: 420, marginRight: 12 }}
              />
              <Button type="primary" loading={approveMutation.isPending} onClick={() => approveMutation.mutate()}>
                审核通过
              </Button>
            </div>

            <div>
              <div style={{ marginBottom: 8 }}>驳回原因（必填）</div>
              <Input
                value={rejectReason}
                onChange={(e) => setRejectReason(e.target.value)}
                placeholder="请输入驳回原因"
                style={{ maxWidth: 420, marginRight: 12 }}
              />
              <Button
                danger
                loading={rejectMutation.isPending}
                disabled={!rejectReason.trim()}
                onClick={() => rejectMutation.mutate()}
              >
                审核驳回
              </Button>
            </div>
          </Space>
        </ProCard>
      ) : null}

      <ProTable<AuditLogItem>
        rowKey="id"
        search={false}
        options={false}
        pagination={false}
        style={{ marginTop: 16 }}
        columns={auditColumns}
        dataSource={auditLogs}
      />

      <div style={{ marginTop: 12 }}>
        <Link to="/admin/logs">查看全局日志</Link>
      </div>
    </PageContainer>
  )
}
