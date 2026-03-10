import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from 'react-router-dom'
import { api } from '../services/api'

export function MerchantProductsPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { data, isLoading, error } = useQuery({
    queryKey: ['merchant-products'],
    queryFn: async () => (await api.products()).data.data as any
  })
  const transitionMutation = useMutation({
    mutationFn: async ({ id, action }: { id: number; action: 'on' | 'off' | 'close' }) => {
      if (action === 'on') return api.productOnShelf(id)
      if (action === 'off') return api.productOffShelf(id)
      return api.productClose(id)
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['merchant-products'] })
    }
  })
  const createOrderMutation = useMutation({
    mutationFn: async (payload: { product_id: number; deal_price_cent: number }) => api.createOrder(payload),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['merchant-products'] })
      void queryClient.invalidateQueries({ queryKey: ['merchant-orders'] })
    }
  })

  if (isLoading) return <p>加载中...</p>
  if (error) return <p className="error">{(error as Error).message}</p>

  return (
    <section className="card">
      <div className="toolbar">
        <h1>商品列表</h1>
        <button onClick={() => navigate('/merchant/products/new')}>新建商品</button>
      </div>
      <table>
        <thead>
          <tr>
            <th>ID</th>
            <th>标题</th>
            <th>状态</th>
            <th>价格</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          {(data.items ?? []).map((item: any) => (
            <tr key={item.id}>
              <td>{item.id}</td>
              <td>{item.title}</td>
              <td>{item.status}</td>
              <td>{item.price_cent}</td>
              <td>
                <Link to={`/merchant/products/${item.id}`}>详情</Link>{' '}
                {(item.status === 'DRAFT' || item.status === 'OFF_SHELF') && (
                  <>
                    <Link to={`/merchant/products/${item.id}/edit`}>编辑</Link>{' '}
                    <button onClick={() => transitionMutation.mutate({ id: item.id, action: 'on' })}>上架</button>{' '}
                  </>
                )}
                {item.status === 'ON_SHELF' && (
                  <>
                    <button onClick={() => transitionMutation.mutate({ id: item.id, action: 'off' })}>下架</button>{' '}
                    <button onClick={() => createOrderMutation.mutate({ product_id: item.id, deal_price_cent: item.price_cent })}>创建订单</button>{' '}
                  </>
                )}
                {(item.status === 'DRAFT' || item.status === 'ON_SHELF' || item.status === 'OFF_SHELF') && (
                  <button onClick={() => transitionMutation.mutate({ id: item.id, action: 'close' })}>关闭</button>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  )
}
