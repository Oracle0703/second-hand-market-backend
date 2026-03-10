import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api } from '../services/api'

export function MerchantProductDetailPage() {
  const { productId = '' } = useParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const detail = useQuery({
    queryKey: ['product-detail', productId],
    queryFn: async () => (await api.productDetail(productId)).data.data.product as any
  })

  const transitionMutation = useMutation({
    mutationFn: async (action: 'on' | 'off' | 'close') => {
      if (action === 'on') return api.productOnShelf(productId)
      if (action === 'off') return api.productOffShelf(productId)
      return api.productClose(productId)
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['product-detail', productId] })
      await queryClient.invalidateQueries({ queryKey: ['merchant-products'] })
    }
  })

  const createOrderMutation = useMutation({
    mutationFn: async () => api.createOrder({ product_id: Number(productId), deal_price_cent: Number(detail.data?.price_cent ?? 0) }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['product-detail', productId] })
      await queryClient.invalidateQueries({ queryKey: ['merchant-orders'] })
    }
  })

  if (detail.isLoading) return <p>加载中...</p>
  if (detail.error) return <p className="error">{(detail.error as Error).message}</p>

  const product = detail.data
  return (
    <section className="card">
      <h1>商品详情</h1>
      <p>ID: {product.id}</p>
      <p>标题: {product.title}</p>
      <p>状态: {product.status}</p>
      <p>价格: {product.price_cent}</p>
      <p>库存: {product.stock}</p>
      <p>图片IDs: {(product.images ?? []).join(', ')}</p>

      <div className="toolbar">
        {(product.status === 'DRAFT' || product.status === 'OFF_SHELF') && (
          <>
            <button onClick={() => navigate(`/merchant/products/${product.id}/edit`)}>编辑</button>
            <button onClick={() => transitionMutation.mutate('on')}>上架</button>
          </>
        )}
        {product.status === 'ON_SHELF' && (
          <>
            <button onClick={() => transitionMutation.mutate('off')}>下架</button>
            <button onClick={() => createOrderMutation.mutate()}>创建订单</button>
          </>
        )}
        {(product.status === 'DRAFT' || product.status === 'ON_SHELF' || product.status === 'OFF_SHELF') && (
          <button onClick={() => transitionMutation.mutate('close')}>关闭商品</button>
        )}
      </div>

      {product.active_order_id ? <Link to={`/merchant/orders/${product.active_order_id}`}>查看关联订单</Link> : null}
      {(transitionMutation.error || createOrderMutation.error) ? (
        <p className="error">{((transitionMutation.error ?? createOrderMutation.error) as Error).message}</p>
      ) : null}
    </section>
  )
}
