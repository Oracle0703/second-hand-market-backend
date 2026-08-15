import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { PageContainer } from '@ant-design/pro-components'
import { Button, Card, Form, Input, InputNumber, Select, Space, Spin, Tag, message } from 'antd'
import { useState } from 'react'
import { api } from '@/services/api'

type CategoryItem = {
  ID?: number
  id?: number
  MerchantID?: number
  merchant_id?: number
  ParentID?: number | null
  parent_id?: number | null
  Level?: number
  level?: number
  Name?: string
  name?: string
  Status?: string
  status?: string
  Sort?: number
  sort?: number
}

type CategoryFormValues = {
  name: string
  sort?: number
  status?: string
}

type EditingState =
  | { type: 'create-root' }
  | { type: 'create-child'; parentID: number }
  | { type: 'edit'; category: NormalizedCategory }
  | null

type NormalizedCategory = {
  id: number
  parent_id?: number | null
  level: 1 | 2
  name: string
  status: string
  sort: number
}

function normalizeCategory(item: CategoryItem): NormalizedCategory {
  return {
    id: Number(item.id ?? item.ID ?? 0),
    parent_id: item.parent_id ?? item.ParentID,
    level: Number(item.level ?? item.Level ?? 1) as 1 | 2,
    name: item.name ?? item.Name ?? '',
    status: item.status ?? item.Status ?? 'ENABLED',
    sort: Number(item.sort ?? item.Sort ?? 0)
  }
}

function statusTag(status: string) {
  return status === 'ENABLED' ? <Tag color="green">启用</Tag> : <Tag>停用</Tag>
}

export function ListPage() {
  const [form] = Form.useForm<CategoryFormValues>()
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState<EditingState>(null)

  const rootsQuery = useQuery({
    queryKey: ['merchant-categories', 1],
    queryFn: async () => ((await api.categories(1, undefined, 'ALL')).data.data.items as CategoryItem[]).map(normalizeCategory)
  })
  const childrenQuery = useQuery({
    queryKey: ['merchant-categories', 2],
    queryFn: async () => ((await api.categories(2, undefined, 'ALL')).data.data.items as CategoryItem[]).map(normalizeCategory)
  })

  const invalidateCategories = async () => {
    await queryClient.invalidateQueries({ queryKey: ['merchant-categories'] })
  }

  const createMutation = useMutation({
    mutationFn: (payload: { level: 1 | 2; parent_id?: number; name: string; sort?: number }) => api.createCategory(payload),
    onSuccess: async () => {
      message.success('分类已创建')
      setEditing(null)
      form.resetFields()
      await invalidateCategories()
    },
    onError: (err) => message.error((err as Error).message)
  })

  const updateMutation = useMutation({
    mutationFn: ({ id, payload }: { id: number; payload: { name: string; sort: number; status: string } }) => api.updateCategory(id, payload),
    onSuccess: async () => {
      message.success('分类已保存')
      setEditing(null)
      form.resetFields()
      await invalidateCategories()
    },
    onError: (err) => message.error((err as Error).message)
  })

  const deleteMutation = useMutation({
    mutationFn: (id: number) => api.deleteCategory(id),
    onSuccess: async () => {
      message.success('分类已删除')
      await invalidateCategories()
    },
    onError: (err) => message.error((err as Error).message)
  })

  const openCreateRoot = () => {
    setEditing({ type: 'create-root' })
    form.setFieldsValue({ name: '', sort: 0, status: 'ENABLED' })
  }

  const openCreateChild = (parentID: number) => {
    setEditing({ type: 'create-child', parentID })
    form.setFieldsValue({ name: '', sort: 0, status: 'ENABLED' })
  }

  const openEdit = (category: NormalizedCategory) => {
    setEditing({ type: 'edit', category })
    form.setFieldsValue({ name: category.name, sort: category.sort, status: category.status })
  }

  const submitForm = async (values: CategoryFormValues) => {
    const name = values.name.trim()
    const sort = Number(values.sort ?? 0)
    if (!editing) return
    if (editing.type === 'create-root') {
      await createMutation.mutateAsync({ level: 1, name, sort })
      return
    }
    if (editing.type === 'create-child') {
      await createMutation.mutateAsync({ level: 2, parent_id: editing.parentID, name, sort })
      return
    }
    await updateMutation.mutateAsync({
      id: editing.category.id,
      payload: { name, sort, status: values.status ?? editing.category.status }
    })
  }

  const roots = rootsQuery.data ?? []
  const children = childrenQuery.data ?? []
  const childrenByParent = children.reduce<Record<number, NormalizedCategory[]>>((result, child) => {
    const parentID = Number(child.parent_id ?? 0)
    if (!result[parentID]) result[parentID] = []
    result[parentID].push(child)
    return result
  }, {})
  const loading = rootsQuery.isLoading || childrenQuery.isLoading
  const saving = createMutation.isPending || updateMutation.isPending

  return (
    <PageContainer
      title="商品分类"
      extra={
        <Button type="primary" onClick={openCreateRoot}>
          新增一级分类
        </Button>
      }
    >
      {editing && (
        <Card size="small" style={{ marginBottom: 16 }}>
          <Form form={form} layout="inline" onFinish={(values) => void submitForm(values)}>
            <Form.Item name="name" label="分类名称" rules={[{ required: true, message: '请输入分类名称' }]}>
              <Input style={{ width: 220 }} />
            </Form.Item>
            <Form.Item name="sort" label="排序">
              <InputNumber min={0} />
            </Form.Item>
            {editing.type === 'edit' && (
              <Form.Item name="status" label="状态">
                <Select
                  style={{ width: 120 }}
                  options={[
                    { value: 'ENABLED', label: '启用' },
                    { value: 'DISABLED', label: '停用' }
                  ]}
                />
              </Form.Item>
            )}
            <Space>
              <Button type="primary" htmlType="submit" aria-label="保存" loading={saving}>
                保存
              </Button>
              <Button onClick={() => setEditing(null)}>取消</Button>
            </Space>
          </Form>
        </Card>
      )}

      <Spin spinning={loading}>
        <Space direction="vertical" style={{ width: '100%' }} size={12}>
          {roots.map((root) => (
            <Card
              key={root.id}
              size="small"
              title={
                <Space>
                  <span>{root.name}</span>
                  {statusTag(root.status)}
                  <span>排序 {root.sort}</span>
                </Space>
              }
              extra={
                <Space>
                  <Button size="small" onClick={() => openCreateChild(root.id)}>
                    新增二级分类
                  </Button>
                  <Button size="small" aria-label={`编辑 ${root.name}`} onClick={() => openEdit(root)}>
                    编辑
                  </Button>
                  <Button size="small" danger aria-label={`删除 ${root.name}`} loading={deleteMutation.isPending} onClick={() => void deleteMutation.mutateAsync(root.id)}>
                    删除
                  </Button>
                </Space>
              }
            >
              <Space direction="vertical" style={{ width: '100%' }}>
                {(childrenByParent[root.id] ?? []).map((child) => (
                  <div key={child.id} style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}>
                    <Space>
                      <span>{child.name}</span>
                      {statusTag(child.status)}
                      <span>排序 {child.sort}</span>
                    </Space>
                    <Space>
                      <Button size="small" aria-label={`编辑 ${child.name}`} onClick={() => openEdit(child)}>
                        编辑
                      </Button>
                      <Button size="small" danger aria-label={`删除 ${child.name}`} loading={deleteMutation.isPending} onClick={() => void deleteMutation.mutateAsync(child.id)}>
                        删除
                      </Button>
                    </Space>
                  </div>
                ))}
              </Space>
            </Card>
          ))}
        </Space>
      </Spin>
    </PageContainer>
  )
}
