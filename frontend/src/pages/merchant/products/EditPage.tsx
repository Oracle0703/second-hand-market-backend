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
import { Alert, Button, Image, Space, Typography, message } from 'antd'
import { useNavigate, useParams } from 'react-router-dom'
import { getStatusText, PRODUCT_CONDITION_META, PRODUCT_STATUS_META, type ProductCondition, type ProductStatus } from '@/constants/status'
import { api } from '@/services/api'
import { centToYuanNumber, yuanToCent } from '@/utils/price'
import { validateUploadFile } from '@/utils/upload'
import { resolveAssetURL } from '@/utils/url'

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
  original_price_cent?: number | null
  stock: number
  reserved_stock: number
  available_stock: number
  condition_level: ProductCondition
  images: number[]
  image_urls?: string[]
}

type ProductEditValues = {
  parent_id?: number
  title: string
  description: string
  price_yuan: number
  original_price_yuan: number
  stock: number
  condition_level: (typeof conditionOptions)[number]
  category_id?: number
}

type EditableImage = {
  fileID: number
  previewURL: string
  fileName: string
  isLocal: boolean
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

export function EditPage() {
  const { productId = '' } = useParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const formRef = useRef<ProFormInstance<ProductEditValues>>()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const imageItemsRef = useRef<EditableImage[]>([])
  const [parentId, setParentID] = useState<number | ''>('')
  const [imageItems, setImageItems] = useState<EditableImage[]>([])

  const detail = useQuery({
    queryKey: ['product-detail', productId],
    queryFn: async () => (await api.productDetail(productId)).data.data.product as ProductDetail
  })

  const level1 = useQuery({
    queryKey: ['categories', 'level1'],
    queryFn: async () => (await api.categories(1)).data.data.items as CategoryItem[],
    staleTime: 5 * 60 * 1000,
    retry: 1
  })
  const level2All = useQuery({
    queryKey: ['categories', 'level2-all'],
    queryFn: async () => (await api.categories(2)).data.data.items as CategoryItem[],
    staleTime: 5 * 60 * 1000,
    retry: 1
  })

  const releaseLocalPreviews = (items: EditableImage[]) => {
    items.forEach((item) => {
      if (item.isLocal && item.previewURL) {
        URL.revokeObjectURL(item.previewURL)
      }
    })
  }

  useEffect(() => {
    if (!detail.data) return
    const selectedCategoryID = Number(detail.data.category_id)
    const matchedLevel2 = (level2All.data ?? []).find((item) => categoryId(item) === selectedCategoryID)
    const pid = matchedLevel2 ? Number(matchedLevel2.ParentID ?? matchedLevel2.parent_id ?? 0) : ''
    setParentID(pid)
    formRef.current?.setFieldsValue({
      title: detail.data.title,
      description: detail.data.description ?? '',
      price_yuan: centToYuanNumber(detail.data.price_cent),
      original_price_yuan: centToYuanNumber(detail.data.original_price_cent ?? detail.data.price_cent),
      stock: Number(detail.data.stock ?? 1),
      condition_level: detail.data.condition_level,
      parent_id: pid || undefined,
      category_id: selectedCategoryID
    })
    const remoteImages = (detail.data.images ?? []).map((fileID, index) => ({
      fileID,
      previewURL: resolveAssetURL(detail.data.image_urls?.[index]),
      fileName: `image-${fileID}`,
      isLocal: false
    }))
    setImageItems((prev) => {
      releaseLocalPreviews(prev)
      return remoteImages
    })
  }, [detail.data, level2All.data])

  const status = detail.data?.status
  const canEditAll = status === 'DRAFT' || status === 'OFF_SHELF'
  const canEditDescImages = canEditAll || status === 'ON_SHELF'

  const updateMutation = useMutation({
    mutationFn: async (values: ProductEditValues) => {
      if (!canEditDescImages) {
        throw new Error('当前状态不可编辑')
      }
      if (imageItems.length === 0) {
        throw new Error('至少保留一张图片')
      }
      const imageIDs = imageItems.map((item) => item.fileID)
      if (canEditAll) {
        if (!values.category_id) {
          throw new Error('请选择二级分类')
        }
        if (Number(values.original_price_yuan) < Number(values.price_yuan)) {
          throw new Error('原价不能低于价格')
        }
        if (Number(values.stock) <= 0) {
          throw new Error('库存数量必须大于0')
        }
        if (Number(values.stock) < Number(detail.data?.reserved_stock ?? 0)) {
          throw new Error('库存数量不能低于已预占数量')
        }
        return api.updateProduct(productId, {
          title: values.title,
          description: values.description,
          category_id: values.category_id,
          price_cent: yuanToCent(values.price_yuan),
          original_price_cent: yuanToCent(values.original_price_yuan),
          stock: Math.max(1, Math.floor(Number(values.stock))),
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
    onSuccess: (fileID, file) => {
      const previewURL = URL.createObjectURL(file)
      setImageItems((prev) => {
        if (prev.length >= 5) {
          URL.revokeObjectURL(previewURL)
          return prev
        }
        return [...prev, { fileID, previewURL, fileName: file.name || `image-${fileID}`, isLocal: true }]
      })
      message.success(`已添加图片：${file.name || `file_id=${fileID}`}`)
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
    const validationError = validateUploadFile(file)
    if (validationError) {
      message.error(validationError)
      return
    }
    if (!canEditDescImages) {
      message.error('当前状态不可编辑图片')
      return
    }
    if (uploadMutation.isPending) {
      message.info('图片上传中，请稍候')
      return
    }
    if (imageItems.length >= 5) {
      message.error('最多上传5张图片')
      return
    }
    uploadMutation.mutate(file)
  }

  useEffect(() => {
    imageItemsRef.current = imageItems
  }, [imageItems])

  useEffect(() => {
    return () => {
      releaseLocalPreviews(imageItemsRef.current)
    }
  }, [])

  const removeImage = (fileID: number) => {
    if (!canEditDescImages) return
    setImageItems((prev) => {
      const current = prev.find((item) => item.fileID === fileID)
      if (current?.isLocal) {
        URL.revokeObjectURL(current.previewURL)
      }
      return prev.filter((item) => item.fileID !== fileID)
    })
  }

  const level2Options = useMemo(() => {
    const pid = Number(parentId)
    if (!pid) return []
    return (level2All.data ?? []).filter((item) => Number(item.ParentID ?? item.parent_id ?? 0) === pid)
  }, [parentId, level2All.data])

  if (detail.isLoading) return <p>加载中...</p>
  if (detail.error) return <p className="error">{(detail.error as Error).message}</p>
  const backToDetail = () => navigate(`/merchant/products/${productId}`)

  return (
    <PageContainer
      title="编辑商品"
      subTitle={`当前状态: ${getStatusText(PRODUCT_STATUS_META, status)}`}
      onBack={backToDetail}
      extra={[
        <Button key="cancel-top" onClick={backToDetail}>
          取消
        </Button>
      ]}
    >
      {!canEditDescImages ? <Alert type="warning" showIcon message="该状态下不可编辑商品" style={{ marginBottom: 16 }} /> : null}

      <ProCard title="图片" style={{ marginBottom: 16 }}>
        <Space wrap>
          {imageItems.length > 0 ? (
            imageItems.map((item) => (
              <div key={item.fileID} style={{ width: 120 }}>
                {item.previewURL ? (
                  <Image
                    width={120}
                    height={120}
                    src={item.previewURL}
                    alt={item.fileName}
                    style={{ objectFit: 'cover', borderRadius: 8 }}
                  />
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
                <div style={{ marginTop: 6, fontSize: 12, color: '#666', wordBreak: 'break-all' }}>file_id: {item.fileID}</div>
                {canEditDescImages ? (
                  <Button type="link" danger size="small" style={{ padding: 0 }} onClick={() => removeImage(item.fileID)}>
                    删除
                  </Button>
                ) : null}
              </div>
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
          <div style={{ marginTop: 8 }}>
            <Typography.Text type="secondary">支持 JPG、PNG、WebP、HEIC、HEIF，原图最大 10 MiB，服务端自动压缩。</Typography.Text>
          </div>
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
            disabled: !canEditDescImages || imageItems.length === 0
          },
          render: (_, dom) => [
            <Button key="cancel-bottom" onClick={backToDetail}>
              取消
            </Button>,
            ...dom
          ]
        }}
      >
        <ProFormText name="title" label="标题" disabled={!canEditAll} rules={[{ required: true, message: '请输入标题' }]} />
        <ProFormTextArea name="description" label="描述" disabled={!canEditDescImages} rules={[{ required: true, message: '请输入描述' }]} />
        <ProFormDigit
          name="price_yuan"
          label="价格(元)"
          min={0.01}
          disabled={!canEditAll}
          fieldProps={{ precision: 2, step: 0.01 }}
          rules={[{ required: true, message: '请输入价格' }]}
        />
        <ProFormDigit
          name="original_price_yuan"
          label="原价(元)"
          min={0.01}
          disabled={!canEditAll}
          fieldProps={{ precision: 2, step: 0.01 }}
          rules={[{ required: true, message: '请输入原价' }]}
        />
        <ProFormDigit
          name="stock"
          label="库存数量"
          min={Math.max(1, Number(detail.data?.reserved_stock ?? 0))}
          disabled={!canEditAll}
          fieldProps={{ precision: 0, step: 1 }}
          rules={[{ required: true, message: '请输入库存数量' }]}
        />
        {canEditAll ? (
          <Typography.Text type="secondary">
            已预占 {detail.data?.reserved_stock ?? 0}，可售 {detail.data?.available_stock ?? 0}
          </Typography.Text>
        ) : null}
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
              const currentCategoryID = Number(formRef.current?.getFieldValue('category_id') ?? 0)
              const stillValid = (level2All.data ?? []).some(
                (item) =>
                  Number(item.ParentID ?? item.parent_id ?? 0) === Number(nextParentID) && categoryId(item) === currentCategoryID
              )
              if (!stillValid) {
                formRef.current?.setFieldValue('category_id', undefined)
              }
            }
          }}
          rules={canEditAll ? [{ required: true, message: '请选择一级分类' }] : []}
        />
        <ProFormSelect
          name="category_id"
          label="二级分类"
          disabled={!canEditAll}
          options={level2Options.map((item) => ({ value: categoryId(item), label: categoryName(item) }))}
          fieldProps={{ loading: level2All.isLoading }}
          rules={canEditAll ? [{ required: true, message: '请选择二级分类' }] : []}
        />
      </ProForm>
    </PageContainer>
  )
}
