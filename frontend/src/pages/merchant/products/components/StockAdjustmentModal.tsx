import { Alert, Form, Input, InputNumber, Modal, Select, message } from 'antd'
import type { ProductStatus } from '@/constants/status'
import { api, type AdjustProductStockPayload } from '@/services/api'
import { STOCK_ADJUSTMENT_TYPE_OPTIONS, type StockAdjustmentType } from '../stock-adjustment'

export type StockAdjustmentProduct = {
  id: number
  title: string
  status: ProductStatus
  stock: number
}

type StockAdjustmentFormValues = {
  adjustment_type: StockAdjustmentType
  quantity: number
  reason: string
}

type StockAdjustmentModalProps = {
  open: boolean
  product: StockAdjustmentProduct | null
  onCancel: () => void
  onSuccess: () => void | Promise<void>
}

export function StockAdjustmentModal({ open, product, onCancel, onSuccess }: StockAdjustmentModalProps) {
  const [form] = Form.useForm<StockAdjustmentFormValues>()
  const currentStock = Number(product?.stock ?? 0)

  const submit = async () => {
    if (!product) return
    const values = await form.validateFields()
    const payload: AdjustProductStockPayload = {
      adjustment_type: values.adjustment_type,
      quantity: Math.floor(Number(values.quantity)),
      reason: values.reason.trim()
    }
    await api.adjustProductStock(product.id, payload)
    message.success('库存调整成功')
    form.resetFields()
    await onSuccess()
  }

  return (
    <Modal
      title={product ? `调整库存 - ${product.title}` : '调整库存'}
      open={open}
      okText="确认调整"
      cancelText="取消"
      onCancel={onCancel}
      onOk={() => void submit()}
      destroyOnHidden
    >
      <Alert type="info" showIcon message={`当前库存：${currentStock}`} style={{ marginBottom: 16 }} />
      <Form<StockAdjustmentFormValues>
        form={form}
        layout="vertical"
        initialValues={{ adjustment_type: 'INCREASE', quantity: 1, reason: '' }}
      >
        <Form.Item name="adjustment_type" label="调整类型" rules={[{ required: true, message: '请选择调整类型' }]}>
          <Select options={STOCK_ADJUSTMENT_TYPE_OPTIONS} />
        </Form.Item>
        <Form.Item
          name="quantity"
          label="调整数量"
          rules={[
            { required: true, message: '请输入调整数量' },
            {
              validator: async (_, value) => {
                const quantity = Math.floor(Number(value))
                const type = form.getFieldValue('adjustment_type')
                if (!Number.isFinite(quantity) || quantity <= 0) throw new Error('调整数量必须大于 0')
                if ((type === 'DECREASE' || type === 'MARK_SOLD') && quantity > currentStock) throw new Error('调整数量不能超过当前库存')
              }
            }
          ]}
        >
          <InputNumber min={1} precision={0} style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item
          name="reason"
          label="调整原因"
          rules={[
            { required: true, message: '请输入调整原因' },
            {
              validator: async (_, value) => {
                const reason = String(value ?? '').trim()
                if (reason.length < 2 || reason.length > 255) throw new Error('调整原因需为 2 到 255 个字符')
              }
            }
          ]}
        >
          <Input.TextArea rows={3} maxLength={255} showCount placeholder="例如：盘点补录、盘点减少、客户线下购买" />
        </Form.Item>
      </Form>
    </Modal>
  )
}
