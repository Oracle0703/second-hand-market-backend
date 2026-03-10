import { FormEvent, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api } from '../services/api'

export function AdminMerchantReviewDetailPage() {
  const { merchantId = '' } = useParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [rejectReason, setRejectReason] = useState('')
  const [approveComment, setApproveComment] = useState('')

  const detailQuery = useQuery({
    queryKey: ['admin-merchant-detail', merchantId],
    queryFn: async () => (await api.adminMerchantReviewDetail(merchantId)).data.data as any
  })

  const approveMutation = useMutation({
    mutationFn: async () => api.adminMerchantApprove(merchantId, approveComment || undefined),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['admin-merchant-detail', merchantId] })
      await queryClient.invalidateQueries({ queryKey: ['admin-merchants'] })
    }
  })

  const rejectMutation = useMutation({
    mutationFn: async () => api.adminMerchantReject(merchantId, rejectReason),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['admin-merchant-detail', merchantId] })
      await queryClient.invalidateQueries({ queryKey: ['admin-merchants'] })
      setRejectReason('')
    }
  })

  const submitReject = (e: FormEvent) => {
    e.preventDefault()
    if (!rejectReason.trim()) return
    rejectMutation.mutate()
  }

  if (detailQuery.isLoading) return <p>加载中...</p>
  if (detailQuery.error) return <p className="error">{(detailQuery.error as Error).message}</p>

  const detail = detailQuery.data
  const merchant = detail.merchant_detail ?? {}
  const auditLogs = detail.audit_logs ?? []
  const status = merchant.review_status

  return (
    <section className="card">
      <div className="toolbar">
        <h1>审核详情</h1>
        <button onClick={() => navigate('/admin/merchants/reviews')}>返回列表</button>
      </div>

      <p>ID: {merchant.id}</p>
      <p>商家编号: {merchant.merchant_no}</p>
      <p>商家名称: {merchant.merchant_name}</p>
      <p>联系人: {merchant.contact_name}</p>
      <p>联系电话: {merchant.contact_phone}</p>
      <p>审核状态: {status}</p>
      {merchant.reject_reason ? <p>驳回原因: {merchant.reject_reason}</p> : null}
      {merchant.license_file_id ? <p>资质文件ID: {merchant.license_file_id}</p> : null}

      {status === 'PENDING' ? (
        <div className="card" style={{ marginTop: 12 }}>
          <h2>审核动作</h2>
          <label>
            通过备注（可选）
            <input value={approveComment} onChange={(e) => setApproveComment(e.target.value)} />
          </label>
          <button onClick={() => approveMutation.mutate()} disabled={approveMutation.isPending}>
            审核通过
          </button>

          <form onSubmit={submitReject}>
            <label>
              驳回原因（必填）
              <input value={rejectReason} onChange={(e) => setRejectReason(e.target.value)} />
            </label>
            <button type="submit" disabled={rejectMutation.isPending || !rejectReason.trim()}>
              审核驳回
            </button>
          </form>
        </div>
      ) : null}

      {(approveMutation.error || rejectMutation.error) ? (
        <p className="error">{((approveMutation.error ?? rejectMutation.error) as Error).message}</p>
      ) : null}

      <h2>审核历史</h2>
      <table>
        <thead>
          <tr>
            <th>时间</th>
            <th>动作</th>
            <th>状态流转</th>
            <th>原因</th>
            <th>操作人</th>
          </tr>
        </thead>
        <tbody>
          {auditLogs.map((log: any) => (
            <tr key={log.id}>
              <td>{log.created_at}</td>
              <td>{log.action}</td>
              <td>{log.from_status || '-'} -&gt; {log.to_status}</td>
              <td>{log.reason || '-'}</td>
              <td>{log.operator_type}#{log.operator_id}</td>
            </tr>
          ))}
        </tbody>
      </table>

      <p style={{ marginTop: 12 }}>
        <Link to="/admin/logs">查看全局日志</Link>
      </p>
    </section>
  )
}
