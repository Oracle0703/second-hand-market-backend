export function buildLoginRedirect(targetPath: string): string {
  return `/pages/login/index?redirect=${encodeURIComponent(targetPath)}`
}

export function resolveLoginRedirect(raw: string | undefined): string {
  if (!raw) return '/pages/me/index'
  const decoded = decodeURIComponent(raw)
  if (!decoded.startsWith('/pages/')) return '/pages/me/index'
  return decoded
}

export function canSubmitIntentWhenLoggedIn(isLogin: boolean): boolean {
  return isLogin
}
