import { FormEvent, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../services/api'

export function MerchantLogsPage() {
  const [action, setAction] = useState('')
  const [resourceType, setResourceType] = useState('')
  const [querySeed, setQuerySeed] = useState(0)

  const logsQuery = useQuery({
    queryKey: ['merchant-logs', action, resourceType, querySeed],
    queryFn: async () => {
      const params: Record<string, string> = {}
      if (action.trim()) params.action = action.trim()
      if (resourceType.trim()) params.resource_type = resourceType.trim()
      return (await api.merchantLogs(params)).data.data as any
    }
  })

  const submitFilter = (e: FormEvent) => {
    e.preventDefault()
    setQuerySeed((v) => v + 1)
  }

  if (logsQuery.isLoading) return <p>加载中...</p>
  if (logsQuery.error) return <p className="error">{(logsQuery.error as Error).message}</p>

  const data = logsQuery.data
  return (
    <section className="card">
      <h1>商家操作日志</h1>
      <form className="toolbar" onSubmit={submitFilter}>
        <label>
          动作
          <input value={action} onChange={(e) => setAction(e.target.value)} placeholder="如 order_create" />
        </label>
        <label>
          资源类型
          <input value={resourceType} onChange={(e) => setResourceType(e.target.value)} placeholder="product/order" />
        </label>
        <button type="submit">筛选</button>
      </form>

      <table>
        <thead>
          <tr>
            <th>时间</th>
            <th>动作</th>
            <th>资源</th>
            <th>状态流转</th>
            <th>结果码</th>
            <th>请求ID</th>
          </tr>
        </thead>
        <tbody>
          {(data.items ?? []).map((item: any) => (
            <tr key={item.id}>
              <td>{item.created_at}</td>
              <td>{item.action}</td>
              <td>{item.resource_type}#{item.resource_id}</td>
              <td>{item.from_status || '-'} -&gt; {item.to_status || '-'}</td>
              <td>{item.result_code}</td>
              <td>{item.request_id}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  )
}
