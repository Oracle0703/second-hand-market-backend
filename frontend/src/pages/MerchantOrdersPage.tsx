import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api } from '../services/api'

export function MerchantOrdersPage() {
  const { data, isLoading, error } = useQuery({
    queryKey: ['merchant-orders'],
    queryFn: async () => (await api.orders()).data.data as any
  })

  if (isLoading) return <p>加载中...</p>
  if (error) return <p className="error">{(error as Error).message}</p>

  return (
    <section className="card">
      <h1>订单列表</h1>
      <table>
        <thead>
          <tr>
            <th>ID</th>
            <th>订单号</th>
            <th>商品</th>
            <th>状态</th>
            <th>成交价</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          {(data.items ?? []).map((item: any) => (
            <tr key={item.id}>
              <td>{item.id}</td>
              <td>{item.order_no}</td>
              <td>{item.product_id}</td>
              <td>{item.status}</td>
              <td>{item.deal_price_cent}</td>
              <td>
                <Link to={`/merchant/orders/${item.id}`}>详情</Link>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  )
}
