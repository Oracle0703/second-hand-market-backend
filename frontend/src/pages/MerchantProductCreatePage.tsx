import { FormEvent, useMemo, useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { api } from '../services/api'

const conditionOptions = ['LIKE_NEW', 'GOOD', 'FAIR', 'POOR'] as const

export function MerchantProductCreatePage() {
  const navigate = useNavigate()
  const [parentId, setParentID] = useState<number | ''>('')
  const [categoryID, setCategoryID] = useState<number | ''>('')
  const [imageIDs, setImageIDs] = useState<number[]>([])
  const [form, setForm] = useState({
    title: '',
    description: '',
    price_cent: 100,
    condition_level: 'GOOD' as (typeof conditionOptions)[number]
  })

  const level1 = useQuery({
    queryKey: ['categories', 'level1'],
    queryFn: async () => (await api.categories(1)).data.data.items as any[]
  })
  const level2 = useQuery({
    queryKey: ['categories', 'level2', parentId],
    enabled: !!parentId,
    queryFn: async () => (await api.categories(2, Number(parentId))).data.data.items as any[]
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
    onSuccess: (fileID) => setImageIDs((prev) => [...prev, fileID])
  })

  const createMutation = useMutation({
    mutationFn: async () =>
      api.createProduct({
        title: form.title,
        description: form.description,
        category_id: Number(categoryID),
        price_cent: Number(form.price_cent),
        condition_level: form.condition_level,
        stock: 1,
        image_file_ids: imageIDs
      }),
    onSuccess: (res) => {
      navigate(`/merchant/products/${res.data.data.product_id}`)
    }
  })

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    createMutation.mutate()
  }

  return (
    <form className="card" onSubmit={onSubmit}>
      <h1>新建商品</h1>
      <label>
        标题
        <input value={form.title} onChange={(e) => setForm((prev) => ({ ...prev, title: e.target.value }))} />
      </label>
      <label>
        描述
        <textarea value={form.description} onChange={(e) => setForm((prev) => ({ ...prev, description: e.target.value }))} />
      </label>
      <label>
        价格(分)
        <input type="number" value={form.price_cent} onChange={(e) => setForm((prev) => ({ ...prev, price_cent: Number(e.target.value) }))} />
      </label>
      <label>
        成色
        <select value={form.condition_level} onChange={(e) => setForm((prev) => ({ ...prev, condition_level: e.target.value as (typeof conditionOptions)[number] }))}>
          {conditionOptions.map((item) => (
            <option key={item} value={item}>
              {item}
            </option>
          ))}
        </select>
      </label>
      <label>
        一级分类
        <select
          value={parentId}
          onChange={(e) => {
            const raw = e.target.value
            setParentID(raw ? Number(raw) : '')
            setCategoryID('')
          }}
        >
          <option value="">请选择</option>
          {(level1.data ?? []).map((item: any) => (
            <option key={item.ID ?? item.id} value={item.ID ?? item.id}>
              {item.Name ?? item.name}
            </option>
          ))}
        </select>
      </label>
      <label>
        二级分类
        <select value={categoryID} onChange={(e) => setCategoryID(e.target.value ? Number(e.target.value) : '')}>
          <option value="">请选择</option>
          {selectedLevel2.map((item: any) => (
            <option key={item.ID ?? item.id} value={item.ID ?? item.id}>
              {item.Name ?? item.name}
            </option>
          ))}
        </select>
      </label>

      <div className="toolbar">
        <span>图片 file_ids: {imageIDs.join(', ') || '无'}</span>
        <button type="button" onClick={() => uploadMutation.mutate()}>
          添加示例图片
        </button>
      </div>

      {createMutation.error ? <p className="error">{(createMutation.error as Error).message}</p> : null}
      <button type="submit" disabled={!categoryID || imageIDs.length === 0 || createMutation.isPending}>
        创建
      </button>
    </form>
  )
}
