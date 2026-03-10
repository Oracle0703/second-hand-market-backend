import { FormEvent, useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useParams } from 'react-router-dom'
import { api } from '../services/api'

const conditionOptions = ['LIKE_NEW', 'GOOD', 'FAIR', 'POOR'] as const

export function MerchantProductEditPage() {
  const { productId = '' } = useParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [parentId, setParentID] = useState<number | ''>('')
  const [form, setForm] = useState({
    title: '',
    description: '',
    price_cent: 0,
    condition_level: 'GOOD' as (typeof conditionOptions)[number],
    category_id: 0,
    image_file_ids: [] as number[]
  })

  const detail = useQuery({
    queryKey: ['product-detail', productId],
    queryFn: async () => (await api.productDetail(productId)).data.data.product as any
  })

  const level1 = useQuery({
    queryKey: ['categories', 'level1'],
    queryFn: async () => (await api.categories(1)).data.data.items as any[]
  })
  const level2All = useQuery({
    queryKey: ['categories', 'level2-all'],
    queryFn: async () => (await api.categories(2)).data.data.items as any[]
  })
  const level2ByParent = useQuery({
    queryKey: ['categories', 'level2', parentId],
    enabled: !!parentId,
    queryFn: async () => (await api.categories(2, Number(parentId))).data.data.items as any[]
  })

  useEffect(() => {
    if (!detail.data) return
    setForm({
      title: detail.data.title,
      description: detail.data.description ?? '',
      price_cent: detail.data.price_cent,
      condition_level: detail.data.condition_level,
      category_id: detail.data.category_id,
      image_file_ids: detail.data.images ?? []
    })
  }, [detail.data])

  useEffect(() => {
    if (!detail.data || !level2All.data) return
    const row = level2All.data.find((item: any) => Number(item.ID ?? item.id) === Number(detail.data.category_id))
    if (!row) return
    setParentID(Number(row.ParentID ?? row.parent_id))
  }, [detail.data, level2All.data])

  const status = detail.data?.status as string | undefined
  const canEditAll = status === 'DRAFT' || status === 'OFF_SHELF'
  const canEditDescImages = canEditAll || status === 'ON_SHELF'

  const updateMutation = useMutation({
    mutationFn: async () => {
      if (!canEditDescImages) {
        throw new Error('当前状态不可编辑')
      }
      if (canEditAll) {
        return api.updateProduct(productId, {
          title: form.title,
          description: form.description,
          category_id: form.category_id,
          price_cent: form.price_cent,
          condition_level: form.condition_level,
          image_file_ids: form.image_file_ids
        })
      }
      return api.updateProduct(productId, {
        description: form.description,
        image_file_ids: form.image_file_ids
      })
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['product-detail', productId] })
      await queryClient.invalidateQueries({ queryKey: ['merchant-products'] })
      navigate(`/merchant/products/${productId}`)
    }
  })

  const uploadMutation = useMutation({
    mutationFn: async () => {
      const presign = await api.presign({
        biz_type: 'PRODUCT_IMAGE',
        file_name: `product-edit-${Date.now()}.jpg`,
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
      setForm((prev) => ({ ...prev, image_file_ids: [...prev.image_file_ids, fileID] }))
    }
  })

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    updateMutation.mutate()
  }

  const level2Options = useMemo(() => level2ByParent.data ?? [], [level2ByParent.data])

  if (detail.isLoading) return <p>加载中...</p>
  if (detail.error) return <p className="error">{(detail.error as Error).message}</p>

  return (
    <form className="card" onSubmit={onSubmit}>
      <h1>编辑商品</h1>
      <p>当前状态: {status}</p>

      <label>
        标题
        <input disabled={!canEditAll} value={form.title} onChange={(e) => setForm((prev) => ({ ...prev, title: e.target.value }))} />
      </label>
      <label>
        描述
        <textarea disabled={!canEditDescImages} value={form.description} onChange={(e) => setForm((prev) => ({ ...prev, description: e.target.value }))} />
      </label>
      <label>
        价格(分)
        <input disabled={!canEditAll} type="number" value={form.price_cent} onChange={(e) => setForm((prev) => ({ ...prev, price_cent: Number(e.target.value) }))} />
      </label>
      <label>
        成色
        <select disabled={!canEditAll} value={form.condition_level} onChange={(e) => setForm((prev) => ({ ...prev, condition_level: e.target.value as (typeof conditionOptions)[number] }))}>
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
          disabled={!canEditAll}
          value={parentId}
          onChange={(e) => {
            const raw = e.target.value
            setParentID(raw ? Number(raw) : '')
            setForm((prev) => ({ ...prev, category_id: 0 }))
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
        <select
          disabled={!canEditAll}
          value={form.category_id || ''}
          onChange={(e) => setForm((prev) => ({ ...prev, category_id: e.target.value ? Number(e.target.value) : 0 }))}
        >
          <option value="">请选择</option>
          {level2Options.map((item: any) => (
            <option key={item.ID ?? item.id} value={item.ID ?? item.id}>
              {item.Name ?? item.name}
            </option>
          ))}
        </select>
      </label>

      <div className="toolbar">
        <span>图片 file_ids: {form.image_file_ids.join(', ') || '无'}</span>
        <button type="button" onClick={() => uploadMutation.mutate()} disabled={!canEditDescImages || uploadMutation.isPending}>
          添加示例图片
        </button>
      </div>

      {!canEditDescImages ? <p>该状态下不可编辑商品。</p> : null}
      {updateMutation.error ? <p className="error">{(updateMutation.error as Error).message}</p> : null}
      <button type="submit" disabled={updateMutation.isPending || !canEditDescImages || form.image_file_ids.length === 0 || (canEditAll && !form.category_id)}>
        保存
      </button>
    </form>
  )
}
