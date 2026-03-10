import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api } from '../services/api'

export function AdminMerchantReviewsPage() {
  const { data, isLoading, error } = useQuery({
    queryKey: ['admin-merchants'],
    queryFn: async () => (await api.adminMerchantReviews()).data.data as any
  })

  if (isLoading) return <p>加载中...</p>
  if (error) return <p className="error">{(error as Error).message}</p>

  return (
    <section className="card">
      <h1>商家审核列表</h1>
      <table>
        <thead>
          <tr>
            <th>ID</th>
            <th>商家</th>
            <th>状态</th>
          </tr>
        </thead>
        <tbody>
          {(data.items ?? []).map((item: any) => (
            <tr key={item.id}>
              <td>{item.id}</td>
              <td>
                <Link to={`/admin/merchants/reviews/${item.id}`}>{item.merchant_name}</Link>
              </td>
              <td>{item.review_status}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  )
}
