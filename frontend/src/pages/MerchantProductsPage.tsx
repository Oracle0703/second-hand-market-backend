import { useMutation } from '@tanstack/react-query'
import { PageContainer, ProTable, type ActionType, type ProColumns } from '@ant-design/pro-components'
import { Button, Space, Tag, message } from 'antd'
import { useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import { getStatusColor, getStatusText, PRODUCT_STATUS_META, toValueEnum, type ProductStatus } from '../constants/status'
import { api } from '../services/api'

type ProductItem = {
  id: number
  title: string
  status: ProductStatus
  price_cent: number
  stock: number
  updated_at: string
}

type ProductListResp = {
  items: ProductItem[]
  total: number
  page: number
  page_size: number
}

export function MerchantProductsPage() {
  const navigate = useNavigate()
  const actionRef = useRef<ActionType>()
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

  const columns: ProColumns<ProductItem>[] = [
    {
      title: 'ID',
      dataIndex: 'id',
      width: 80,
      search: false
    },
    {
      title: '标题',
      dataIndex: 'title',
      ellipsis: true,
      formItemProps: {
        label: '关键词'
      }
    },
    {
      title: '状态',
      dataIndex: 'status',
      valueType: 'select',
      valueEnum: toValueEnum(PRODUCT_STATUS_META),
      render: (_, row) => <Tag color={getStatusColor(PRODUCT_STATUS_META, row.status)}>{getStatusText(PRODUCT_STATUS_META, row.status)}</Tag>
    },
    {
      title: '价格(分)',
      dataIndex: 'price_cent',
      search: false,
      width: 120
    },
    {
      title: '更新时间',
      dataIndex: 'updated_at',
      valueType: 'dateTime',
      search: false,
      width: 180
    },
    {
      title: '操作',
      key: 'actions',
      search: false,
      width: 320,
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
        pagination={{ pageSize: 20 }}
        request={async (params) => {
          try {
            const query: Record<string, string | number> = {
              page: params.current ?? 1,
              page_size: params.pageSize ?? 20
            }
            if (params.title) query.keyword = params.title
            if (params.status) query.status = params.status as string
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
    </PageContainer>
  )
}
