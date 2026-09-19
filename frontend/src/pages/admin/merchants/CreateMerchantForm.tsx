import { useState } from 'react'
import { Button, Form, Input, Modal, message } from 'antd'
import { api } from '@/services/api'
import { PasswordInput } from '@/components/PasswordInput'
import { passwordHelp, passwordPattern } from '@/utils/password'

type Values = {
  merchant_name: string
  contact_name: string
  phone: string
  username: string
  password: string
}
export function CreateMerchantForm({ onCreated }: { onCreated: () => void }) {
  const [open, setOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const [form] = Form.useForm<Values>()
  return (
    <>
      <Button type="primary" onClick={() => setOpen(true)}>
        创建商户账号
      </Button>
      <Modal
        title="创建商户账号"
        open={open}
        confirmLoading={saving}
        onCancel={() => {
          setOpen(false)
          form.resetFields()
        }}
        onOk={() => form.submit()}
      >
        <Form
          name="create-merchant"
          form={form}
          layout="vertical"
          onFinish={async (values) => {
            setSaving(true)
            try {
              await api.adminCreateMerchant(values)
              message.success('账号已创建，商户首次登录须修改密码')
              setOpen(false)
              form.resetFields()
              onCreated()
            } catch (error) {
              message.error((error as Error).message)
            } finally {
              setSaving(false)
            }
          }}
        >
          <Form.Item
            name="merchant_name"
            label="商户名称"
            rules={[{ required: true }, { max: 128 }]}
          >
            <Input />
          </Form.Item>
          <Form.Item
            name="contact_name"
            label="联系人"
            rules={[{ required: true }, { max: 64 }]}
          >
            <Input />
          </Form.Item>
          <Form.Item
            name="phone"
            label="联系电话"
            rules={[{ required: true }, { max: 20 }]}
          >
            <Input />
          </Form.Item>
          <Form.Item
            name="username"
            label="登录账号"
            rules={[
              { required: true },
              {
                pattern: /^[A-Za-z0-9_-]{3,64}$/,
                message: '3–64 位字母、数字、下划线或短横线'
              }
            ]}
          >
            <Input autoComplete="off" />
          </Form.Item>
          <Form.Item
            name="password"
            label="初始密码"
            extra={`${passwordHelp}。请在创建前复制保存，系统不提供密码找回展示。`}
            rules={[
              { required: true },
              { pattern: passwordPattern, message: passwordHelp }
            ]}
          >
            <PasswordInput />
          </Form.Item>
        </Form>
      </Modal>
    </>
  )
}
