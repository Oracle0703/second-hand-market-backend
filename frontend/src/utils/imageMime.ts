const supportedImageMIMEs = new Set([
  'image/jpeg',
  'image/png',
  'image/webp',
  'image/heic',
  'image/heif'
])

const supportedImageExtensions = [
  ['.jpg', 'image/jpeg'],
  ['.jpeg', 'image/jpeg'],
  ['.png', 'image/png'],
  ['.webp', 'image/webp'],
  ['.heic', 'image/heic'],
  ['.heif', 'image/heif']
] as const

export const MAX_IMAGE_UPLOAD_BYTES = 40 * 1024 * 1024

function normalizeDeclaredMIME(type?: string) {
  const raw = type?.trim().toLowerCase()
  if (raw === 'image/jpg') return 'image/jpeg'
  return raw ?? ''
}

function mimeFromName(name: string) {
  const lowerName = name.toLowerCase()
  return supportedImageExtensions.find(([ext]) => lowerName.endsWith(ext))?.[1]
}

export function normalizeImageMIME(file: Pick<File, 'name' | 'type'>) {
  const raw = normalizeDeclaredMIME(file.type)
  if (supportedImageMIMEs.has(raw)) return raw

  return mimeFromName(file.name) ?? 'image/jpeg'
}

export function validateImageFileForUpload(file: Pick<File, 'name' | 'type' | 'size'>) {
  if (file.size <= 0) {
    return '图片文件为空或读取失败，请重新选择图片'
  }
  if (file.size > MAX_IMAGE_UPLOAD_BYTES) {
    return '图片原图超过 40MB，请先压缩后再上传'
  }
  const raw = normalizeDeclaredMIME(file.type)
  const hasSupportedMIME = supportedImageMIMEs.has(raw)
  const hasSupportedExtension = Boolean(mimeFromName(file.name))
  if (!hasSupportedMIME && !hasSupportedExtension) {
    return '图片格式不支持，请上传 JPG、PNG、WebP、HEIC、HEIF'
  }
  return null
}
