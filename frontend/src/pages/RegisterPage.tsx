import { FormEvent, useRef, useState, type ChangeEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../services/api'

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
  const [form, setForm] = useState({
    merchant_name: '',
    contact_name: '',
    phone: '',
    username: '',
    password: ''
  })
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

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    if (!licenseFileID) {
      setError('请先上传营业执照图片（最多1张）')
      return
    }
    try {
      await api.register({ ...form, license_file_id: licenseFileID })
      navigate('/login')
    } catch (err) {
      setError((err as Error).message)
    }
  }

  return (
    <main className="auth-page">
      <form className="card" onSubmit={onSubmit}>
        <h1>商家注册</h1>
        {Object.entries(form).map(([key, value]) => (
          <label key={key}>
            {key}
            <input value={value} onChange={(e) => setForm((prev) => ({ ...prev, [key]: e.target.value }))} />
          </label>
        ))}
        <label>
          营业执照（最多1张）
          <input
            ref={fileInputRef}
            type="file"
            accept="image/jpeg,image/png,image/webp,image/heic,image/heif,image/*"
            capture="environment"
            style={{ display: 'none' }}
            onChange={onSelectLicense}
          />
          <button type="button" onClick={() => fileInputRef.current?.click()} disabled={uploading}>
            {uploading ? '上传中...' : '选择并上传图片'}
          </button>
          {licenseFileID ? <span>已上传：{licenseFileName || '执照图片'}（file_id: {licenseFileID}）</span> : <span>未上传</span>}
        </label>
        {error ? <p className="error">{error}</p> : null}
        <button type="submit" disabled={uploading || !licenseFileID}>
          提交注册
        </button>
      </form>
    </main>
  )
}
