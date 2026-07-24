import { useEffect, useState } from 'react'
import { Form, InputNumber, Modal, Space, Typography, message } from 'antd'
import { api } from '@/services/api'
import { centToYuanNumber, centToYuanText, yuanToCent } from '@/utils/price'

export type OrderableProduct = {
  id: number
  title: string
  price_cent: number
  available_stock: number
}

type OrderFormValues = {
  quantity: number
  unit_price_yuan: number
}

type Props = {
  open: boolean
  product: OrderableProduct | null
  onCancel: () => void
  onCreated: () => void | Promise<void>
}

export function CreateOrderModal({ open, product, onCancel, onCreated }: Props) {
  const [form] = Form.useForm<OrderFormValues>()
  const [submitting, setSubmitting] = useState(false)
  const quantity = Form.useWatch('quantity', form) ?? 1
  const unitPriceYuan = Form.useWatch('unit_price_yuan', form) ?? 0
  const totalCent = yuanToCent(unitPriceYuan) * Math.floor(Number(quantity || 0))

  useEffect(() => {
    if (!open || !product) return
    form.setFieldsValue({
      quantity: 1,
      unit_price_yuan: centToYuanNumber(product.price_cent)
    })
  }, [form, open, product])

  const submit = async (values: OrderFormValues) => {
    if (!product) return
    setSubmitting(true)
    try {
      await api.createOrder({
        product_id: product.id,
        quantity: Math.floor(values.quantity),
        deal_price_cent: yuanToCent(values.unit_price_yuan)
      })
    } catch (error) {
      setSubmitting(false)
      message.error((error as Error).message)
      return
    }

    message.success('订单创建成功')
    setSubmitting(false)
    onCancel()
    try {
      await onCreated()
    } catch {
      message.warning('订单已创建，列表刷新失败，请手动刷新')
    }
  }

  return (
    <Modal
      title="创建订单"
      open={open}
      okText="创建"
      cancelText="取消"
      onCancel={onCancel}
      onOk={() => form.submit()}
      confirmLoading={submitting}
      closable={!submitting}
      maskClosable={!submitting}
      cancelButtonProps={{ disabled: submitting }}
      okButtonProps={{ disabled: submitting || !product || product.available_stock <= 0 }}
      destroyOnHidden
    >
      <Space direction="vertical" size={4} style={{ marginBottom: 16 }}>
        <Typography.Text strong>{product?.title ?? '-'}</Typography.Text>
        <Typography.Text type="secondary">可售库存：{product?.available_stock ?? 0}</Typography.Text>
      </Space>
      <Form<OrderFormValues> form={form} layout="vertical" onFinish={submit}>
        <Form.Item
          name="quantity"
          label="数量"
          rules={[
            { required: true, message: '请输入数量' },
            {
              validator: (_, value) => {
                if (!Number.isInteger(value) || value <= 0) return Promise.reject(new Error('数量必须为正整数'))
                if (product && value > product.available_stock) return Promise.reject(new Error('数量不能超过可售库存'))
                return Promise.resolve()
              }
            }
          ]}
        >
          <InputNumber min={1} max={Math.max(1, product?.available_stock ?? 1)} precision={0} step={1} style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item
          name="unit_price_yuan"
          label="单件成交价(元)"
          rules={[{ required: true, message: '请输入单件成交价' }, { type: 'number', min: 0.01, message: '单件成交价必须大于 0' }]}
        >
          <InputNumber min={0.01} precision={2} step={0.01} style={{ width: '100%' }} />
        </Form.Item>
        <Typography.Text strong>订单总价：{centToYuanText(totalCent)} 元</Typography.Text>
      </Form>
    </Modal>
  )
}
