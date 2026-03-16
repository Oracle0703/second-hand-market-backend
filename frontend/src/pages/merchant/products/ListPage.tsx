import { useMutation, useQuery } from '@tanstack/react-query'
import { PageContainer, ProTable, type ActionType, type ProColumns } from '@ant-design/pro-components'
import { Button, Empty, Image, Modal, Space, Spin, Tag, message } from 'antd'
import { useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { getStatusColor, getStatusText, PRODUCT_STATUS_META, toValueEnum, type ProductStatus } from '@/constants/status'
import { api } from '@/services/api'
import { centToYuanText } from '@/utils/price'
import { resolveAssetURL } from '@/utils/url'

type ProductItem = {
  id: number
  title: string
  status: ProductStatus
  price_cent: number
  stock: number
  updated_at: string
  category_level1_id?: number | null
  category_level1_name?: string | null
  category_level2_id?: number | null
  category_level2_name?: string | null
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
  const level1 = useQuery({
    queryKey: ['categories', 'level1'],
    queryFn: async () => (await api.categories(1)).data.data.items as CategoryItem[]
  })
  const transitionMutation = useMutation({
    mutationFn: async ({ id, action }: { id: number; action: 'on' | 'off' | 'close' }) => {
      if (action === 'on') return api.productOnShelf(id)
      if (action === 'off') return api.productOffShelf(id)
      return api.productClose(id)
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
      width: 110,
      render: (_, row) => (
        <Button type="link" onClick={() => void handleViewImages(row)}>
          查看
        </Button>
      )
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
      width: 200,
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
          {(row.status === 'DRAFT' || row.status === 'ON_SHELF' || row.status === 'OFF_SHELF') && (
            <Button
              type="link"
              danger
              loading={transitionMutation.isPending}
              onClick={() => transitionMutation.mutate({ id: row.id, action: 'close' })}
            >
              关闭
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
