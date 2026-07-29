const supportedImageMIMEs = new Set([
  'image/jpeg',
  'image/png',
  'image/webp',
  'image/heic',
  'image/heif'
])

export function normalizeImageMIME(file: Pick<File, 'name' | 'type'>) {
  const raw = file.type?.trim().toLowerCase()
  if (raw === 'image/jpg') return 'image/jpeg'
  if (supportedImageMIMEs.has(raw)) return raw

  const name = file.name.toLowerCase()
  if (name.endsWith('.png')) return 'image/png'
  if (name.endsWith('.webp')) return 'image/webp'
  if (name.endsWith('.heic')) return 'image/heic'
  if (name.endsWith('.heif')) return 'image/heif'
  return 'image/jpeg'
}
