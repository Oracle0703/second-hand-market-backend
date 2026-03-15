import { useMemo, useRef, useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  PageContainer,
  ProCard,
  ProForm,
  ProFormDigit,
  ProFormSelect,
  ProFormText,
  ProFormTextArea,
  type ProFormInstance
} from '@ant-design/pro-components'
import { Button, Space, Tag, message } from 'antd'
import { useNavigate } from 'react-router-dom'
import { PRODUCT_CONDITION_META, type ProductCondition } from '../constants/status'
import { api } from '../services/api'

const conditionOptions: ProductCondition[] = ['LIKE_NEW', 'GOOD', 'FAIR', 'POOR']

type CategoryItem = {
  ID?: number
  id?: number
  Name?: string
  name?: string
}

type ProductCreateValues = {
  parent_id?: number
  category_id?: number
  title: string
  description: string
  price_cent: number
  condition_level: ProductCondition
}

function categoryId(item: CategoryItem) {
  return Number(item.ID ?? item.id ?? 0)
}

function categoryName(item: CategoryItem) {
  return item.Name ?? item.name ?? ''
}

export function MerchantProductCreatePage() {
  const navigate = useNavigate()
  const formRef = useRef<ProFormInstance>()
  const [parentId, setParentID] = useState<number | ''>('')
  const [categoryID, setCategoryID] = useState<number | ''>('')
  const [imageIDs, setImageIDs] = useState<number[]>([])

  const level1 = useQuery({
    queryKey: ['categories', 'level1'],
    queryFn: async () => (await api.categories(1)).data.data.items as CategoryItem[]
  })
  const level2 = useQuery({
    queryKey: ['categories', 'level2', parentId],
    enabled: !!parentId,
    queryFn: async () => (await api.categories(2, Number(parentId))).data.data.items as CategoryItem[]
  })

  const selectedLevel2 = useMemo(() => level2.data ?? [], [level2.data])

  const uploadMutation = useMutation({
    mutationFn: async () => {
      const presign = await api.presign({
        biz_type: 'PRODUCT_IMAGE',
        file_name: `product-${Date.now()}.jpg`,
        file_size: 1024,
        mime_type: 'image/jpeg'
      })
      await api.confirmUpload({
        file_id: presign.data.data.file_id,
        object_key: presign.data.data.object_key
      })
      return presign.data.data.file_id as number
    },
    onSuccess: (fileID) => {
      setImageIDs((prev) => [...prev, fileID])
      message.success(`已添加图片 file_id=${fileID}`)
    },
    onError: (err) => {
      message.error((err as Error).message)
    }
  })

  const createMutation = useMutation({
    mutationFn: async (payload: ProductCreateValues) =>
      api.createProduct({
        title: payload.title,
        description: payload.description,
        category_id: Number(categoryID),
        price_cent: Number(payload.price_cent),
        condition_level: payload.condition_level,
        stock: 1,
        image_file_ids: imageIDs
      }),
    onSuccess: (res) => {
      message.success('商品创建成功')
      navigate(`/merchant/products/${res.data.data.product_id}`)
    },
    onError: (err) => {
      message.error((err as Error).message)
    }
  })

  const onFinish = async (values: ProductCreateValues) => {
    if (!categoryID) {
      message.error('请选择二级分类')
      return false
    }
    if (imageIDs.length === 0) {
      message.error('请先添加至少一张图片')
      return false
    }
    await createMutation.mutateAsync(values)
    return true
  }

  return (
    <PageContainer title="新建商品">
      <ProCard title="图片" style={{ marginBottom: 16 }}>
        <Space wrap>
          {imageIDs.length > 0 ? imageIDs.map((id) => <Tag key={id}>file_id: {id}</Tag>) : <span>暂无图片</span>}
        </Space>
        <div style={{ marginTop: 12 }}>
          <Button type="dashed" loading={uploadMutation.isPending} onClick={() => uploadMutation.mutate()}>
            添加示例图片
          </Button>
        </div>
      </ProCard>

      <ProForm<ProductCreateValues>
        formRef={formRef}
        layout="vertical"
        initialValues={{
          title: '',
          description: '',
          price_cent: 100,
          condition_level: 'GOOD'
        }}
        onFinish={onFinish}
        submitter={{
          searchConfig: {
            submitText: '创建'
          },
          submitButtonProps: {
            loading: createMutation.isPending,
            disabled: !categoryID || imageIDs.length === 0
          }
        }}
      >
        <ProFormText name="title" label="标题" rules={[{ required: true, message: '请输入标题' }]} />
        <ProFormTextArea name="description" label="描述" rules={[{ required: true, message: '请输入描述' }]} />
        <ProFormDigit
          name="price_cent"
          label="价格(分)"
          min={1}
          rules={[{ required: true, message: '请输入价格' }]}
          fieldProps={{ precision: 0 }}
        />
        <ProFormSelect
          name="condition_level"
          label="成色"
          options={conditionOptions.map((item) => ({ label: PRODUCT_CONDITION_META[item].text, value: item }))}
          rules={[{ required: true, message: '请选择成色' }]}
        />
        <ProFormSelect
          name="parent_id"
          label="一级分类"
          options={(level1.data ?? []).map((item) => ({ value: categoryId(item), label: categoryName(item) }))}
          fieldProps={{
            value: parentId || undefined,
            loading: level1.isLoading,
            onChange: (value) => {
              const nextParentID = value ? Number(value) : ''
              setParentID(nextParentID)
              setCategoryID('')
              formRef.current?.setFieldValue('category_id', undefined)
            }
          }}
          rules={[{ required: true, message: '请选择一级分类' }]}
        />
        <ProFormSelect
          name="category_id"
          label="二级分类"
          options={selectedLevel2.map((item) => ({ value: categoryId(item), label: categoryName(item) }))}
          fieldProps={{
            value: categoryID || undefined,
            loading: level2.isLoading,
            onChange: (value) => setCategoryID(value ? Number(value) : '')
          }}
          rules={[{ required: true, message: '请选择二级分类' }]}
        />
      </ProForm>
    </PageContainer>
  )
}
