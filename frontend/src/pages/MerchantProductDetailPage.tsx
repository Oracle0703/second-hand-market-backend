import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { PageContainer, ProCard, ProDescriptions } from '@ant-design/pro-components'
import { Alert, Button, Space, Tag, message } from 'antd'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { getStatusColor, getStatusText, PRODUCT_STATUS_META, type ProductStatus } from '../constants/status'
import { api } from '../services/api'

type ProductDetail = {
  id: number
  title: string
  description: string
  status: ProductStatus
  category_id: number
  price_cent: number
  condition_level: string
  stock: number
  images: number[]
  active_order_id?: number
}

export function MerchantProductDetailPage() {
  const { productId = '' } = useParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const detail = useQuery({
    queryKey: ['product-detail', productId],
    queryFn: async () => (await api.productDetail(productId)).data.data.product as ProductDetail
  })

  const transitionMutation = useMutation({
    mutationFn: async (action: 'on' | 'off' | 'close') => {
      if (action === 'on') return api.productOnShelf(productId)
      if (action === 'off') return api.productOffShelf(productId)
      return api.productClose(productId)
    },
    onSuccess: async () => {
      message.success('状态更新成功')
      await queryClient.invalidateQueries({ queryKey: ['product-detail', productId] })
      await queryClient.invalidateQueries({ queryKey: ['merchant-products'] })
    },
    onError: (err) => {
      message.error((err as Error).message)
    }
  })

  const createOrderMutation = useMutation({
    mutationFn: async () => api.createOrder({ product_id: Number(productId), deal_price_cent: Number(detail.data?.price_cent ?? 0) }),
    onSuccess: async () => {
      message.success('订单创建成功')
      await queryClient.invalidateQueries({ queryKey: ['product-detail', productId] })
      await queryClient.invalidateQueries({ queryKey: ['merchant-orders'] })
    },
    onError: (err) => {
      message.error((err as Error).message)
    }
  })

  if (detail.isLoading) return <p>加载中...</p>
  if (detail.error) return <p className="error">{(detail.error as Error).message}</p>
  if (!detail.data) return <p>暂无数据</p>

  const product = detail.data
  return (
    <PageContainer
      title="商品详情"
      subTitle={product.title}
      extra={[
        (product.status === 'DRAFT' || product.status === 'OFF_SHELF') && (
          <Button key="edit" onClick={() => navigate(`/merchant/products/${product.id}/edit`)}>
            编辑
          </Button>
        ),
        (product.status === 'DRAFT' || product.status === 'OFF_SHELF') && (
          <Button key="on" type="primary" loading={transitionMutation.isPending} onClick={() => transitionMutation.mutate('on')}>
            上架
          </Button>
        ),
        product.status === 'ON_SHELF' && (
          <Button key="off" loading={transitionMutation.isPending} onClick={() => transitionMutation.mutate('off')}>
            下架
          </Button>
        ),
        product.status === 'ON_SHELF' && (
          <Button key="order" type="primary" loading={createOrderMutation.isPending} onClick={() => createOrderMutation.mutate()}>
            创建订单
          </Button>
        ),
        (product.status === 'DRAFT' || product.status === 'ON_SHELF' || product.status === 'OFF_SHELF') && (
          <Button key="close" danger loading={transitionMutation.isPending} onClick={() => transitionMutation.mutate('close')}>
            关闭商品
          </Button>
        )
      ].filter(Boolean)}
    >
      {(transitionMutation.error || createOrderMutation.error) ? <Alert type="error" showIcon message={((transitionMutation.error ?? createOrderMutation.error) as Error).message} style={{ marginBottom: 16 }} /> : null}

      <ProDescriptions<ProductDetail>
        column={2}
        columns={[
          { title: 'ID', dataIndex: 'id' },
          {
            title: '状态',
            dataIndex: 'status',
            render: (_, row) => <Tag color={getStatusColor(PRODUCT_STATUS_META, row.status)}>{getStatusText(PRODUCT_STATUS_META, row.status)}</Tag>
          },
          { title: '价格(分)', dataIndex: 'price_cent' },
          { title: '库存', dataIndex: 'stock' },
          { title: '成色', dataIndex: 'condition_level' },
          { title: '分类ID', dataIndex: 'category_id' },
          { title: '描述', dataIndex: 'description', span: 2 },
          {
            title: '图片',
            dataIndex: 'images',
            span: 2,
            render: (_, row) => (
              <Space wrap>
                {(row.images ?? []).map((id) => (
                  <Tag key={id}>file_id: {id}</Tag>
                ))}
              </Space>
            )
          }
        ]}
        dataSource={product}
      />

      {product.active_order_id ? (
        <ProCard title="关联订单" style={{ marginTop: 16 }}>
          <Link to={`/merchant/orders/${product.active_order_id}`}>查看订单 #{product.active_order_id}</Link>
        </ProCard>
      ) : null}
    </PageContainer>
  )
}
