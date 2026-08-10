import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { PageContainer, ProCard, ProDescriptions } from '@ant-design/pro-components'
import { Alert, Button, Image, Space, Tag, message } from 'antd'
import { useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { getStatusColor, getStatusText, PRODUCT_STATUS_META, type ProductStatus } from '@/constants/status'
import { api } from '@/services/api'
import { centToYuanText } from '@/utils/price'
import { resolveAssetURL } from '@/utils/url'
import { StockAdjustmentModal } from './components/StockAdjustmentModal'
import { canAdjustProductStock } from './stock-adjustment'

type ProductDetail = {
  id: number
  title: string
  description: string
  status: ProductStatus
  category_id: number
  price_cent: number
  original_price_cent?: number | null
  condition_level: string
  stock: number
  images: number[]
  image_urls?: string[]
  active_order_id?: number
}

export function DetailPage() {
  const { productId = '' } = useParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [stockAdjustOpen, setStockAdjustOpen] = useState(false)
  const [markSoldAllRemaining, setMarkSoldAllRemaining] = useState(false)
  const detail = useQuery({
    queryKey: ['product-detail', productId],
    queryFn: async () => (await api.productDetail(productId)).data.data.product as ProductDetail
  })

  const transitionMutation = useMutation({
    mutationFn: async (action: 'on' | 'off') => {
      if (action === 'on') return api.productOnShelf(productId)
      return api.productOffShelf(productId)
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
  const backToList = () => navigate('/merchant/products')
  const actionButtons = [
    <Button key="back-list" onClick={backToList}>
      返回列表
    </Button>,
    canAdjustProductStock(product.status) && (
      <Button
        key="adjust-stock"
        onClick={() => {
          setMarkSoldAllRemaining(false)
          setStockAdjustOpen(true)
        }}
      >
        调整库存
      </Button>
    ),
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
    (product.status === 'ON_SHELF' || product.status === 'OFF_SHELF') && product.stock > 0 ? (
      <Button
        key="mark-sold"
        danger
        onClick={() => {
          setMarkSoldAllRemaining(true)
          setStockAdjustOpen(true)
        }}
      >
        设为售罄
      </Button>
    ) : null
  ].filter(Boolean)

  return (
    <PageContainer title="商品详情" subTitle={product.title} onBack={backToList} extra={actionButtons}>
      {(product.status === 'DRAFT' || product.status === 'OFF_SHELF') ? (
        <Alert
          type="info"
          showIcon
          message="当前商品未上架，请点击“上架”后再对外展示。"
          style={{ marginBottom: 16 }}
        />
      ) : null}

      {actionButtons.length > 0 ? (
        <ProCard style={{ marginBottom: 16 }}>
          <Space wrap>{actionButtons}</Space>
        </ProCard>
      ) : null}

      {(transitionMutation.error || createOrderMutation.error) ? <Alert type="error" showIcon message={((transitionMutation.error ?? createOrderMutation.error) as Error).message} style={{ marginBottom: 16 }} /> : null}

      <StockAdjustmentModal
        open={stockAdjustOpen}
        product={{ id: product.id, title: product.title, status: product.status, stock: product.stock }}
        markSoldAllRemaining={markSoldAllRemaining}
        onCancel={() => {
          setMarkSoldAllRemaining(false)
          setStockAdjustOpen(false)
        }}
        onSuccess={async () => {
          setMarkSoldAllRemaining(false)
          setStockAdjustOpen(false)
          await queryClient.invalidateQueries({ queryKey: ['product-detail', productId] })
          await queryClient.invalidateQueries({ queryKey: ['merchant-products'] })
        }}
      />

      <ProDescriptions<ProductDetail>
        column={2}
        columns={[
          { title: 'ID', dataIndex: 'id' },
          {
            title: '状态',
            dataIndex: 'status',
            render: (_, row) => <Tag color={getStatusColor(PRODUCT_STATUS_META, row.status)}>{getStatusText(PRODUCT_STATUS_META, row.status)}</Tag>
          },
          { title: '价格(元)', dataIndex: 'price_cent', render: (_, row) => centToYuanText(row.price_cent) },
          { title: '原价(元)', dataIndex: 'original_price_cent', render: (_, row) => (row.original_price_cent ? centToYuanText(row.original_price_cent) : '-') },
          { title: '库存', dataIndex: 'stock' },
          { title: '成色', dataIndex: 'condition_level' },
          { title: '分类ID', dataIndex: 'category_id' },
          { title: '描述', dataIndex: 'description', span: 2 },
          {
            title: '图片',
            dataIndex: 'images',
            span: 2,
            render: (_, row) => {
              const urls = (row.image_urls ?? []).map(resolveAssetURL)
              return (
              <Space wrap>
                {(row.images ?? []).map((id, index) => {
                  const url = urls[index] || ''
                  return (
                    <div key={id} style={{ width: 120 }}>
                      {url ? (
                        <Image width={120} height={120} src={url} alt={`product-${id}`} style={{ objectFit: 'cover', borderRadius: 8 }} />
                      ) : (
                        <div
                          style={{
                            width: 120,
                            height: 120,
                            borderRadius: 8,
                            border: '1px solid #eee',
                            background: '#fafafa',
                            color: '#999',
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            fontSize: 12
                          }}
                        >
                          无预览
                        </div>
                      )}
                      <div style={{ marginTop: 6 }}>
                        <Tag>file_id: {id}</Tag>
                      </div>
                    </div>
                  )
                })}
              </Space>
              )
            }
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
