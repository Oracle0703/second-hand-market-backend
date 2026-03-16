const DEFAULT_API_BASE_URL = 'http://localhost:8080/api/v1'

function getAPIOrigin() {
  const base = import.meta.env.VITE_API_BASE_URL ?? DEFAULT_API_BASE_URL
  try {
    return new URL(base).origin
  } catch {
    return ''
  }
}

export function resolveAssetURL(rawURL?: string | null) {
  const url = String(rawURL ?? '').trim()
  if (!url) return ''
  if (/^(https?:)?\/\//i.test(url) || url.startsWith('data:') || url.startsWith('blob:')) {
    return url
  }
  const origin = getAPIOrigin()
  if (!origin) return url
  if (url.startsWith('/')) return `${origin}${url}`
  return `${origin}/${url.replace(/^\/+/, '')}`
}
