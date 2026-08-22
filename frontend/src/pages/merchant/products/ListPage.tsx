import { useMutation, useQuery } from '@tanstack/react-query'
import { PageContainer, ProTable, type ActionType, type ProColumns } from '@ant-design/pro-components'
import { Button, Empty, Image, Modal, Space, Spin, Tag, message } from 'antd'
import { useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { getStatusColor, getStatusText, PRODUCT_STATUS_META, toValueEnum, type ProductStatus } from '@/constants/status'
import { api } from '@/services/api'
import { centToYuanText } from '@/utils/price'
import { resolveAssetURL } from '@/utils/url'
import { StockAdjustmentModal, type StockAdjustmentProduct } from './components/StockAdjustmentModal'
import { canAdjustProductStock } from './stock-adjustment'

type ProductItem = {
  id: number
  title: string
  status: ProductStatus
  price_cent: number
  original_price_cent?: number | null
  stock: number
  updated_at: string
  category_level1_id?: number | null
  category_level1_name?: string | null
  category_level2_id?: number | null
  category_level2_name?: string | null
  cover_url?: string | null
}

type ProductListResp = {
  items: ProductItem[]
  total: number
  page: number
  page_size: number
}

type ProductDetailPayload = {
  id: number
  title: string
  images: number[]
  image_urls?: string[]
}

type CategoryItem = {
  ID?: number
  id?: number
  Name?: string
  name?: string
}

function categoryId(item: CategoryItem) {
  return Number(item.ID ?? item.id ?? 0)
}

function categoryName(item: CategoryItem) {
  return item.Name ?? item.name ?? ''
}

export function ListPage() {
  const navigate = useNavigate()
  const actionRef = useRef<ActionType>()
  const [previewOpen, setPreviewOpen] = useState(false)
  const [previewLoading, setPreviewLoading] = useState(false)
  const [previewTitle, setPreviewTitle] = useState('')
  const [previewImageURLs, setPreviewImageURLs] = useState<string[]>([])
  const [previewImageIDs, setPreviewImageIDs] = useState<number[]>([])
  const [stockAdjustProduct, setStockAdjustProduct] = useState<StockAdjustmentProduct | null>(null)
  const [markSoldAllRemaining, setMarkSoldAllRemaining] = useState(false)
  const level1 = useQuery({
    queryKey: ['categories', 'level1'],
    queryFn: async () => (await api.categories(1)).data.data.items as CategoryItem[]
  })
  const transitionMutation = useMutation({
    mutationFn: async ({ id, action }: { id: number; action: 'on' | 'off' }) => {
      if (action === 'on') return api.productOnShelf(id)
      return api.productOffShelf(id)
    },
    onSuccess: () => {
      actionRef.current?.reload()
    },
    onError: (err) => {
      message.error((err as Error).message)
    }
  })
  const createOrderMutation = useMutation({
    mutationFn: async (payload: { product_id: number; deal_price_cent: number }) => api.createOrder(payload),
    onSuccess: () => {
      actionRef.current?.reload()
    },
    onError: (err) => {
      message.error((err as Error).message)
    }
  })
  const deleteMutation = useMutation({
    mutationFn: async (productID: number) => api.deleteProduct(productID),
    onSuccess: () => {
      message.success('商品已删除')
      actionRef.current?.reload()
    },
    onError: (err) => {
      message.error((err as Error).message)
    }
  })

  const handleViewImages = async (row: ProductItem) => {
    setPreviewOpen(true)
    setPreviewLoading(true)
    setPreviewTitle(row.title)
    setPreviewImageURLs([])
    setPreviewImageIDs([])
    try {
      const res = await api.productDetail(row.id)
      const payload = res.data.data.product as ProductDetailPayload
      setPreviewTitle(payload.title || row.title)
      setPreviewImageURLs((payload.image_urls ?? []).map(resolveAssetURL).filter(Boolean))
      setPreviewImageIDs(payload.images ?? [])
    } catch (err) {
      message.error((err as Error).message)
    } finally {
      setPreviewLoading(false)
    }
  }

  const handleDeleteProduct = (row: ProductItem) => {
    Modal.confirm({
      title: '删除商品',
      content: '删除后不可恢复，并会清理该商品关联的图片文件。确定删除吗？',
      okText: '删除',
      okType: 'danger',
      cancelText: '取消',
      onOk: async () => {
        await deleteMutation.mutateAsync(row.id)
      }
    })
  }

  const columns: ProColumns<ProductItem>[] = [
    {
      title: 'ID',
      dataIndex: 'id',
      width: 80,
      fixed: 'left',
      hideInTable: true,
      search: false
    },
    {
      title: '商品名',
      dataIndex: 'title',
      width: 120,
      fixed: 'left',
      ellipsis: true,
      formItemProps: {
        label: '关键词'
      }
    },
    {
      title: '上传图片',
      key: 'images',
      search: false,
      width: 120,
      render: (_, row) => {
        const coverURL = resolveAssetURL(row.cover_url)
        return (
          <Space direction="vertical" size={4} align="center">
            {coverURL ? (
              <Image
                width={56}
                height={56}
                src={coverURL}
                alt={`${row.title}首图`}
                preview={false}
                style={{ objectFit: 'cover', borderRadius: 6 }}
              />
            ) : (
              <span style={{ color: '#999' }}>暂无图片</span>
            )}
            <Button type="link" size="small" style={{ padding: 0 }} onClick={() => void handleViewImages(row)}>
              查看更多
            </Button>
          </Space>
        )
      }
    },
    {
      title: '一级分类',
      dataIndex: 'category_level1_id',
      valueType: 'select',
      hideInTable: true,
      fieldProps: {
        options: (level1.data ?? []).map((item) => ({ value: categoryId(item), label: categoryName(item) })),
        loading: level1.isLoading,
        showSearch: true,
        optionFilterProp: 'label'
      }
    },
    
    {
      title: '一级分类',
      dataIndex: 'category_level1_name',
      search: false,
      width: 80,
      render: (_, row) => row.category_level1_name || '-'
    },
    {
      title: '二级分类',
      dataIndex: 'category_level2_name',
      search: false,
      width: 80,
      render: (_, row) => row.category_level2_name || '-'
    },
    {
      title: '价格(元)',
      dataIndex: 'price_cent',
      search: false,
      width: 100,
      render: (_, row) => centToYuanText(row.price_cent)
    },
    {
      title: '原价(元)',
      dataIndex: 'original_price_cent',
      search: false,
      width: 100,
      render: (_, row) => (row.original_price_cent ? centToYuanText(row.original_price_cent) : '-')
    },
    {
      title: '库存',
      dataIndex: 'stock',
      search: false,
      width: 80
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 96,
      valueType: 'select',
      valueEnum: toValueEnum(PRODUCT_STATUS_META),
      render: (_, row) => <Tag color={getStatusColor(PRODUCT_STATUS_META, row.status)}>{getStatusText(PRODUCT_STATUS_META, row.status)}</Tag>
    },
    {
      title: '更新时间',
      dataIndex: 'updated_at',
      valueType: 'dateTime',
      search: false,
      width: 160
    },
    {
      title: '操作',
      key: 'actions',
      search: false,
      width: 320,
      fixed: 'right',
      render: (_, row) => (
        <Space size={0} wrap>
          <Button type="link" onClick={() => navigate(`/merchant/products/${row.id}`)}>
            详情
          </Button>
          {(row.status === 'DRAFT' || row.status === 'OFF_SHELF') && (
            <>
              <Button type="link" onClick={() => navigate(`/merchant/products/${row.id}/edit`)}>
                编辑
              </Button>
              <Button
                type="link"
                loading={transitionMutation.isPending}
                onClick={() => transitionMutation.mutate({ id: row.id, action: 'on' })}
              >
                上架
              </Button>
            </>
          )}
          {row.status === 'ON_SHELF' && (
            <>
              <Button
                type="link"
                loading={transitionMutation.isPending}
                onClick={() => transitionMutation.mutate({ id: row.id, action: 'off' })}
              >
                下架
              </Button>
              <Button
                type="link"
                loading={createOrderMutation.isPending}
                onClick={() => createOrderMutation.mutate({ product_id: row.id, deal_price_cent: row.price_cent })}
              >
                创建订单
              </Button>
            </>
          )}
          {canAdjustProductStock(row.status) && (
            <Button
              type="link"
              onClick={() => {
                setMarkSoldAllRemaining(false)
                setStockAdjustProduct({ id: row.id, title: row.title, status: row.status, stock: row.stock })
              }}
            >
              调整库存
            </Button>
          )}
          {(row.status === 'ON_SHELF' || row.status === 'OFF_SHELF') && row.stock > 0 ? (
            <Button
              type="link"
              danger
              onClick={() => {
                setMarkSoldAllRemaining(true)
                setStockAdjustProduct({ id: row.id, title: row.title, status: row.status, stock: row.stock })
              }}
            >
              设为售罄
            </Button>
          ) : null}
          {(row.status === 'DRAFT' || row.status === 'OFF_SHELF') && (
            <Button type="link" danger loading={deleteMutation.isPending} onClick={() => handleDeleteProduct(row)}>
              删除
            </Button>
          )}
        </Space>
      )
    }
  ]

  return (
    <PageContainer title="商品列表">
      <ProTable<ProductItem>
        actionRef={actionRef}
        rowKey="id"
        columns={columns}
        scroll={{ x: 1400 }}
        pagination={{ pageSize: 20 }}
        request={async (params) => {
          try {
            const query: Record<string, string | number> = {
              page: params.current ?? 1,
              page_size: params.pageSize ?? 20
            }
            if (params.title) query.keyword = params.title
            if (params.status) query.status = params.status as string
            if (params.category_level1_id) query.category_level1_id = Number(params.category_level1_id)
            const res = await api.products(query)
            const payload = res.data.data as ProductListResp
            return {
              data: payload.items,
              total: payload.total,
              success: true
            }
          } catch (err) {
            message.error((err as Error).message)
            return {
              data: [],
              total: 0,
              success: false
            }
          }
        }}
        toolBarRender={() => [
          <Button key="new" type="primary" onClick={() => navigate('/merchant/products/new')}>
            新建商品
          </Button>
        ]}
      />
      <StockAdjustmentModal
        open={Boolean(stockAdjustProduct)}
        product={stockAdjustProduct}
        markSoldAllRemaining={markSoldAllRemaining}
        onCancel={() => {
          setMarkSoldAllRemaining(false)
          setStockAdjustProduct(null)
        }}
        onSuccess={async () => {
          setMarkSoldAllRemaining(false)
          setStockAdjustProduct(null)
          actionRef.current?.reload()
        }}
      />
      <Modal
        title={`上传图片 - ${previewTitle}`}
        open={previewOpen}
        onCancel={() => setPreviewOpen(false)}
        footer={null}
        width={760}
      >
        {previewLoading ? (
          <div style={{ padding: '24px 0', textAlign: 'center' }}>
            <Spin />
          </div>
        ) : previewImageURLs.length > 0 ? (
          <Space wrap size={12}>
            <Image.PreviewGroup>
              {previewImageURLs.map((url, idx) => (
                <Image key={`${url}-${idx}`} width={120} height={120} src={url} style={{ objectFit: 'cover' }} />
              ))}
            </Image.PreviewGroup>
          </Space>
        ) : previewImageIDs.length > 0 ? (
          <Space wrap>
            {previewImageIDs.map((id) => (
              <Tag key={id}>file_id: {id}</Tag>
            ))}
          </Space>
        ) : (
          <Empty description="当前商品暂无上传图片" />
        )}
      </Modal>
    </PageContainer>
  )
}
