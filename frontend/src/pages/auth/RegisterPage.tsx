import { LoginFormPage, ProFormText } from '@ant-design/pro-components'
import { Alert, Button, Space, Typography, message } from 'antd'
import { useRef, useState, type ChangeEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api } from '@/services/api'

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

export function RegisterPage() {
  const navigate = useNavigate()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [licenseFileID, setLicenseFileID] = useState<number | null>(null)
  const [licenseFileName, setLicenseFileName] = useState('')
  const [uploading, setUploading] = useState(false)
  const [error, setError] = useState('')

  const onSelectLicense = async (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    e.target.value = ''
    if (!file) return
    setError('')
    setUploading(true)
    try {
      const mimeType = normalizeImageMIME(file)
      const presign = await api.presign({
        biz_type: 'MERCHANT_LICENSE',
        file_name: file.name || `license-${Date.now()}.jpg`,
        file_size: file.size,
        mime_type: mimeType
      })
      const formData = new FormData()
      formData.append('file_id', String(presign.data.data.file_id))
      formData.append('object_key', String(presign.data.data.object_key))
      formData.append('file', file)
      await api.uploadFile(formData)
      setLicenseFileID(Number(presign.data.data.file_id))
      setLicenseFileName(file.name || 'license.jpg')
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setUploading(false)
    }
  }

  const onFinish = async (values: {
    merchant_name: string
    contact_name: string
    phone: string
    username: string
    password: string
  }) => {
    setError('')
    if (!licenseFileID) {
      setError('请先上传营业执照图片（最多1张）')
      return false
    }
    try {
      await api.register({
        merchant_name: values.merchant_name,
        contact_name: values.contact_name,
        phone: values.phone,
        username: values.username,
        password: values.password,
        license_file_id: licenseFileID
      })
      message.success('注册申请已提交')
      navigate('/login')
      return true
    } catch (err) {
      setError((err as Error).message)
      return false
    }
  }

  return (
    <LoginFormPage
      title="广汉市瑞扬家具经营部"
      subTitle="商家后台管理系统"
      onFinish={onFinish}
      submitter={{
        searchConfig: {
          submitText: '提交注册'
        },
        submitButtonProps: {
          loading: uploading
        }
      }}
      actions={
        <Space size={8}>
          <span>已有商家账号？</span>
          <Link to="/login">
            <Button type="link" style={{ paddingInline: 0 }}>
              去登录
            </Button>
          </Link>
        </Space>
      }
      containerStyle={{ backgroundColor: '#f5f7fa' }}
    >
      <ProFormText
        name="merchant_name"
        label="商家名称"
        rules={[{ required: true, message: '请输入商家名称' }]}
        fieldProps={{ autoComplete: 'organization' }}
      />
      <ProFormText
        name="contact_name"
        label="联系人姓名"
        rules={[{ required: true, message: '请输入联系人姓名' }]}
        fieldProps={{ autoComplete: 'name' }}
      />
      <ProFormText
        name="phone"
        label="联系电话"
        rules={[{ required: true, message: '请输入联系电话' }]}
        fieldProps={{ autoComplete: 'tel' }}
      />
      <ProFormText
        name="username"
        label="登录账号"
        rules={[{ required: true, message: '请输入登录账号' }]}
        fieldProps={{ autoComplete: 'username' }}
      />
      <ProFormText.Password
        name="password"
        label="登录密码"
        rules={[
          { required: true, message: '请输入登录密码' },
          { min: 8, message: '密码长度至少8位' }
        ]}
        fieldProps={{ autoComplete: 'new-password' }}
      />
      <div>
        <p style={{ marginBottom: 8, color: '#1f2937', fontWeight: 500 }}>营业执照（最多1张）</p>
        <input
          ref={fileInputRef}
          type="file"
          accept="image/jpeg,image/png,image/webp,image/heic,image/heif,image/*"
          style={{ display: 'none' }}
          onChange={onSelectLicense}
        />
        <Space direction="vertical" size={4}>
          <Button type="default" onClick={() => fileInputRef.current?.click()} disabled={uploading}>
            {uploading ? '上传中...' : '选择并上传图片'}
          </Button>
          <Typography.Text type={licenseFileID ? 'success' : 'secondary'}>
            {licenseFileID ? `已上传：${licenseFileName || '执照图片'}（file_id: ${licenseFileID}）` : '未上传'}
          </Typography.Text>
        </Space>
      </div>
      {error ? <Alert type="error" showIcon message={error} /> : null}
    </LoginFormPage>
  )
}
