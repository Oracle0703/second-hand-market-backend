declare const __API_BASE_URL__: string

const DEFAULT_API_BASE_URL = 'https://market.meaningful.ink/api/v1'

function runtimeAPIBaseURL(): string {
  return (typeof __API_BASE_URL__ === 'string' && __API_BASE_URL__.trim()) || DEFAULT_API_BASE_URL
}

function httpOrigin(rawURL: string): string {
  const match = rawURL.trim().match(/^(https?:\/\/[^/?#]+)/i)
  return match?.[1] ?? ''
}

export function resolveAssetURL(rawURL?: string | null, apiBaseURL = runtimeAPIBaseURL()): string {
  const url = String(rawURL ?? '').trim()
  if (!url) return ''
  if (/^(?:https?:\/\/|data:image\/|blob:|wxfile:\/\/|ttfile:\/\/|\/\/)/i.test(url)) return url
  if (/^[a-z][a-z0-9+.-]*:/i.test(url)) return ''

  const uploadPath = url === '/uploads' || url.startsWith('/uploads/')
    ? url
    : url.startsWith('uploads/')
      ? `/${url}`
      : ''
  if (!uploadPath) return url

  const origin = httpOrigin(apiBaseURL)
  return origin ? `${origin}${uploadPath}` : url
}
