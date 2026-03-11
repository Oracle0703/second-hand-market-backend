import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api } from '../services/api'

export function MerchantIntentsPage() {
  const [status, setStatus] = useState('')
  const [keyword, setKeyword] = useState('')
  const params = useMemo(() => {
    const p: Record<string, string> = {}
    if (status) p.status = status
    if (keyword.trim()) p.keyword = keyword.trim()
    return p
  }, [status, keyword])

  const query = useQuery({
    queryKey: ['merchant-intents', params],
    queryFn: async () => (await api.merchantIntents(params)).data.data as any
  })

  if (query.isLoading) return <p>加载中...</p>
  if (query.error) return <p className="error">{(query.error as Error).message}</p>

  return (
    <section className="card">
      <h1>意向线索</h1>
      <div className="toolbar">
        <select value={status} onChange={(e) => setStatus(e.target.value)}>
          <option value="">全部状态</option>
          <option value="NEW">NEW</option>
          <option value="CONTACTED">CONTACTED</option>
          <option value="CLOSED">CLOSED</option>
        </select>
        <input value={keyword} onChange={(e) => setKeyword(e.target.value)} placeholder="搜索商品或联系方式" />
      </div>

      <table>
        <thead>
          <tr>
            <th>ID</th>
            <th>意向单号</th>
            <th>商品</th>
            <th>状态</th>
            <th>联系方式</th>
            <th>创建时间</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          {(query.data.items ?? []).map((item: any) => (
            <tr key={item.id}>
              <td>{item.id}</td>
              <td>{item.intent_no}</td>
              <td>{item.product_title}</td>
              <td>{item.status}</td>
              <td>{item.contact_phone || item.contact_wechat || '-'}</td>
              <td>{item.created_at}</td>
              <td>
                <Link to={`/merchant/intents/${item.id}`}>详情</Link>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  )
}
