import { useEffect, useMemo, useRef, useState, type ChangeEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
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
import { Alert, Button, Space, Tag, message } from 'antd'
import { useNavigate, useParams } from 'react-router-dom'
import { getStatusText, PRODUCT_CONDITION_META, PRODUCT_STATUS_META, type ProductCondition, type ProductStatus } from '../constants/status'
import { api } from '../services/api'

const conditionOptions: ProductCondition[] = ['LIKE_NEW', 'GOOD', 'FAIR', 'POOR']

type CategoryItem = {
  ID?: number
  id?: number
  Name?: string
  name?: string
  ParentID?: number
  parent_id?: number
}

type ProductDetail = {
  id: number
  title: string
  description: string
  status: ProductStatus
  category_id: number
  price_cent: number
  condition_level: ProductCondition
  images: number[]
}

type ProductEditValues = {
  parent_id?: number
  title: string
  description: string
  price_cent: number
  condition_level: (typeof conditionOptions)[number]
  category_id?: number
}

function categoryId(item: CategoryItem) {
  return Number(item.ID ?? item.id ?? 0)
}

function categoryName(item: CategoryItem) {
  return item.Name ?? item.name ?? ''
}

function normalizeImageMIME(file: File) {
  const raw = file.type?.toLowerCase()
  if (raw === 'image/jpg') return 'image/jpeg'
  if (raw) return raw
  const name = file.name.toLowerCase()
  if (name.endsWith('.png')) return 'image/png'
  if (name.endsWith('.webp')) return 'image/webp'
  if (name.endsWith('.heic')) return 'image/heic'
  if (name.endsWith('.heif')) return 'image/heif'
  return 'image/jpeg'
}

export function MerchantProductEditPage() {
  const { productId = '' } = useParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const formRef = useRef<ProFormInstance<ProductEditValues>>()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [parentId, setParentID] = useState<number | ''>('')
  const [imageIDs, setImageIDs] = useState<number[]>([])

  const detail = useQuery({
    queryKey: ['product-detail', productId],
    queryFn: async () => (await api.productDetail(productId)).data.data.product as ProductDetail
  })

  const level1 = useQuery({
    queryKey: ['categories', 'level1'],
    queryFn: async () => (await api.categories(1)).data.data.items as CategoryItem[]
  })
  const level2All = useQuery({
    queryKey: ['categories', 'level2-all'],
    queryFn: async () => (await api.categories(2)).data.data.items as CategoryItem[]
  })
  const level2ByParent = useQuery({
    queryKey: ['categories', 'level2', parentId],
    enabled: !!parentId,
    queryFn: async () => (await api.categories(2, Number(parentId))).data.data.items as CategoryItem[]
  })

  useEffect(() => {
    if (!detail.data) return
    formRef.current?.setFieldsValue({
      title: detail.data.title,
      description: detail.data.description ?? '',
      price_cent: detail.data.price_cent,
      condition_level: detail.data.condition_level,
      category_id: detail.data.category_id
    })
    setImageIDs(detail.data.images ?? [])
  }, [detail.data])

  useEffect(() => {
    if (!detail.data || !level2All.data) return
    const row = level2All.data.find((item) => categoryId(item) === Number(detail.data.category_id))
    if (!row) return
    const pid = Number(row.ParentID ?? row.parent_id ?? 0)
    setParentID(pid)
    formRef.current?.setFieldValue('parent_id', pid)
  }, [detail.data, level2All.data])

  const status = detail.data?.status
  const canEditAll = status === 'DRAFT' || status === 'OFF_SHELF'
  const canEditDescImages = canEditAll || status === 'ON_SHELF'

  const updateMutation = useMutation({
    mutationFn: async (values: ProductEditValues) => {
      if (!canEditDescImages) {
        throw new Error('当前状态不可编辑')
      }
      if (imageIDs.length === 0) {
        throw new Error('至少保留一张图片')
      }
      if (canEditAll) {
        if (!values.category_id) {
          throw new Error('请选择二级分类')
        }
        return api.updateProduct(productId, {
          title: values.title,
          description: values.description,
          category_id: values.category_id,
          price_cent: Number(values.price_cent),
          condition_level: values.condition_level,
          image_file_ids: imageIDs
        })
      }
      return api.updateProduct(productId, {
        description: values.description,
        image_file_ids: imageIDs
      })
    },
    onSuccess: async () => {
      message.success('保存成功')
      await queryClient.invalidateQueries({ queryKey: ['product-detail', productId] })
      await queryClient.invalidateQueries({ queryKey: ['merchant-products'] })
      navigate(`/merchant/products/${productId}`)
    },
    onError: (err) => {
      message.error((err as Error).message)
    }
  })

  const uploadMutation = useMutation({
    mutationFn: async (file: File) => {
      const mimeType = normalizeImageMIME(file)
      const presign = await api.presign({
        biz_type: 'PRODUCT_IMAGE',
        file_name: file.name || `product-edit-${Date.now()}.jpg`,
        file_size: file.size,
        mime_type: mimeType
      })
      const formData = new FormData()
      formData.append('file_id', String(presign.data.data.file_id))
      formData.append('object_key', String(presign.data.data.object_key))
      formData.append('file', file)
      await api.uploadFile(formData)
      return presign.data.data.file_id as number
    },
    onSuccess: (fileID) => {
      setImageIDs((prev) => [...prev, fileID].slice(0, 5))
      message.success(`已添加图片 file_id=${fileID}`)
    },
    onError: (err) => {
      message.error((err as Error).message)
    }
  })

  const onFinish = async (values: ProductEditValues) => {
    await updateMutation.mutateAsync(values)
    return true
  }

  const onSelectImage = (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    e.target.value = ''
    if (!file) return
    if (imageIDs.length >= 5) {
      message.error('最多上传5张图片')
      return
    }
    uploadMutation.mutate(file)
  }

  const level2Options = useMemo(() => level2ByParent.data ?? [], [level2ByParent.data])

  if (detail.isLoading) return <p>加载中...</p>
  if (detail.error) return <p className="error">{(detail.error as Error).message}</p>

  return (
    <PageContainer title="编辑商品" subTitle={`当前状态: ${getStatusText(PRODUCT_STATUS_META, status)}`}>
      {!canEditDescImages ? <Alert type="warning" showIcon message="该状态下不可编辑商品" style={{ marginBottom: 16 }} /> : null}

      <ProCard title="图片" style={{ marginBottom: 16 }}>
        <Space wrap>
          {imageIDs.length > 0 ? (
            imageIDs.map((id) => (
              <Tag
                key={id}
                closable={canEditDescImages}
                onClose={(e) => {
                  e.preventDefault()
                  if (!canEditDescImages) return
                  setImageIDs((prev) => prev.filter((x) => x !== id))
                }}
              >
                file_id: {id}
              </Tag>
            ))
          ) : (
            <span>暂无图片</span>
          )}
        </Space>
        <div style={{ marginTop: 12 }}>
          <input
            ref={fileInputRef}
            type="file"
            accept="image/jpeg,image/png,image/webp,image/heic,image/heif,image/*"
            capture="environment"
            style={{ display: 'none' }}
            onChange={onSelectImage}
          />
          <Button
            type="dashed"
            onClick={() => fileInputRef.current?.click()}
            loading={uploadMutation.isPending}
            disabled={!canEditDescImages}
          >
            选择并上传图片
          </Button>
        </div>
      </ProCard>

      <ProForm<ProductEditValues>
        formRef={formRef}
        layout="vertical"
        onFinish={onFinish}
        submitter={{
          searchConfig: {
            submitText: '保存'
          },
          submitButtonProps: {
            loading: updateMutation.isPending,
            disabled: !canEditDescImages || imageIDs.length === 0
          }
        }}
      >
        <ProFormText name="title" label="标题" disabled={!canEditAll} rules={[{ required: true, message: '请输入标题' }]} />
        <ProFormTextArea name="description" label="描述" disabled={!canEditDescImages} rules={[{ required: true, message: '请输入描述' }]} />
        <ProFormDigit name="price_cent" label="价格(分)" min={1} disabled={!canEditAll} fieldProps={{ precision: 0 }} rules={[{ required: true, message: '请输入价格' }]} />
        <ProFormSelect
          name="condition_level"
          label="成色"
          disabled={!canEditAll}
          options={conditionOptions.map((item) => ({ label: PRODUCT_CONDITION_META[item].text, value: item }))}
          rules={[{ required: true, message: '请选择成色' }]}
        />
        <ProFormSelect
          name="parent_id"
          label="一级分类"
          disabled={!canEditAll}
          options={(level1.data ?? []).map((item) => ({ value: categoryId(item), label: categoryName(item) }))}
          fieldProps={{
            value: parentId || undefined,
            loading: level1.isLoading,
            onChange: (value) => {
              const nextParentID = value ? Number(value) : ''
              setParentID(nextParentID)
              formRef.current?.setFieldValue('category_id', undefined)
            }
          }}
          rules={canEditAll ? [{ required: true, message: '请选择一级分类' }] : []}
        />
        <ProFormSelect
          name="category_id"
          label="二级分类"
          disabled={!canEditAll}
          options={level2Options.map((item) => ({ value: categoryId(item), label: categoryName(item) }))}
          fieldProps={{ loading: level2ByParent.isLoading }}
          rules={canEditAll ? [{ required: true, message: '请选择二级分类' }] : []}
        />
      </ProForm>
    </PageContainer>
  )
}
