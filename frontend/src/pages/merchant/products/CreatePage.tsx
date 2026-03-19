import { useEffect, useMemo, useRef, useState, type ChangeEvent } from 'react'
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
import { Button, Image, Space, message } from 'antd'
import { useNavigate } from 'react-router-dom'
import { PRODUCT_CONDITION_META, type ProductCondition } from '@/constants/status'
import { api } from '@/services/api'
import { yuanToCent } from '@/utils/price'

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
  price_yuan: number
  original_price_yuan: number
  stock: number
  condition_level: ProductCondition
}

type UploadedImage = {
  fileID: number
  previewURL: string
  fileName: string
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

export function CreatePage() {
  const navigate = useNavigate()
  const formRef = useRef<ProFormInstance>()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const uploadedImagesRef = useRef<UploadedImage[]>([])
  const [parentId, setParentID] = useState<number | ''>('')
  const [categoryID, setCategoryID] = useState<number | ''>('')
  const [uploadedImages, setUploadedImages] = useState<UploadedImage[]>([])

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
    mutationFn: async (file: File) => {
      const mimeType = normalizeImageMIME(file)
      const presign = await api.presign({
        biz_type: 'PRODUCT_IMAGE',
        file_name: file.name || `product-${Date.now()}.jpg`,
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
      setUploadedImages((prev) => {
        if (prev.length >= 5) {
          URL.revokeObjectURL(previewURL)
          return prev
        }
        return [...prev, { fileID, previewURL, fileName: file.name || `image-${fileID}` }]
      })
      message.success(`已添加图片：${file.name || `file_id=${fileID}`}`)
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
        price_cent: yuanToCent(payload.price_yuan),
        original_price_cent: yuanToCent(payload.original_price_yuan),
        condition_level: payload.condition_level,
        stock: Math.max(1, Math.floor(Number(payload.stock))),
        image_file_ids: uploadedImages.map((item) => item.fileID)
      }),
    onSuccess: (res) => {
      message.success('商品创建成功，当前为草稿状态，请点击上架后在售')
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
    if (uploadedImages.length === 0) {
      message.error('请先添加至少一张图片')
      return false
    }
    if (Number(values.original_price_yuan) < Number(values.price_yuan)) {
      message.error('原价不能低于价格')
      return false
    }
    if (Number(values.stock) <= 0) {
      message.error('库存数量必须大于0')
      return false
    }
    await createMutation.mutateAsync(values)
    return true
  }

  const onSelectImage = (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    e.target.value = ''
    if (!file) return
    if (uploadMutation.isPending) {
      message.info('图片上传中，请稍候')
      return
    }
    if (uploadedImages.length >= 5) {
      message.error('最多上传5张图片')
      return
    }
    uploadMutation.mutate(file)
  }

  useEffect(() => {
    uploadedImagesRef.current = uploadedImages
  }, [uploadedImages])

  useEffect(() => {
    return () => {
      uploadedImagesRef.current.forEach((item) => {
        URL.revokeObjectURL(item.previewURL)
      })
    }
  }, [])

  const removeImage = (fileID: number) => {
    setUploadedImages((prev) => {
      const current = prev.find((item) => item.fileID === fileID)
      if (current) {
        URL.revokeObjectURL(current.previewURL)
      }
      return prev.filter((item) => item.fileID !== fileID)
    })
  }

  const backToList = () => {
    navigate('/merchant/products')
  }

  return (
    <PageContainer
      title="新建商品"
      onBack={backToList}
      extra={[
        <Button key="cancel" onClick={backToList}>
          取消
        </Button>
      ]}
    >
      <ProCard title="图片" style={{ marginBottom: 16 }}>
        <Space wrap>
          {uploadedImages.length > 0 ? (
            uploadedImages.map((item) => (
              <div key={item.fileID} style={{ width: 120 }}>
                <Image
                  width={120}
                  height={120}
                  src={item.previewURL}
                  alt={item.fileName}
                  style={{ objectFit: 'cover', borderRadius: 8 }}
                />
                <div style={{ marginTop: 6, fontSize: 12, color: '#666', wordBreak: 'break-all' }}>
                  file_id: {item.fileID}
                </div>
                <Button type="link" danger size="small" style={{ padding: 0 }} onClick={() => removeImage(item.fileID)}>
                  删除
                </Button>
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
          <Button type="dashed" loading={uploadMutation.isPending} onClick={() => fileInputRef.current?.click()}>
            选择并上传图片
          </Button>
        </div>
      </ProCard>

      <ProForm<ProductCreateValues>
        formRef={formRef}
        layout="vertical"
        initialValues={{
          title: '',
          description: '',
          price_yuan: 1,
          original_price_yuan: 1,
          stock: 1,
          condition_level: 'GOOD'
        }}
        onFinish={onFinish}
        submitter={{
          searchConfig: {
            submitText: '创建'
          },
          submitButtonProps: {
            loading: createMutation.isPending,
            disabled: !categoryID || uploadedImages.length === 0
          },
          render: (_, dom) => [
            <Button key="form-cancel" onClick={backToList}>
              取消
            </Button>,
            ...dom
          ]
        }}
      >
        <ProFormText name="title" label="标题" rules={[{ required: true, message: '请输入标题' }]} />
        <ProFormTextArea name="description" label="描述" rules={[{ required: true, message: '请输入描述' }]} />
        <ProFormDigit
          name="price_yuan"
          label="价格(元)"
          min={0.01}
          rules={[{ required: true, message: '请输入价格' }]}
          fieldProps={{ precision: 2, step: 0.01 }}
        />
        <ProFormDigit
          name="original_price_yuan"
          label="原价(元)"
          min={0.01}
          rules={[{ required: true, message: '请输入原价' }]}
          fieldProps={{ precision: 2, step: 0.01 }}
        />
        <ProFormDigit
          name="stock"
          label="库存数量"
          min={1}
          rules={[{ required: true, message: '请输入库存数量' }]}
          fieldProps={{ precision: 0, step: 1 }}
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
