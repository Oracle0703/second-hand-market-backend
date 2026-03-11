export const GUEST_ACCESSIBLE_PAGES = [
  'pages/home/index',
  'pages/product/list/index',
  'pages/product/detail/index',
  'pages/favorite/index',
  'pages/history/index',
  'pages/search/index',
  'pages/category/index',
  'pages/me/index',
  'pages/login/index'
]

export function isGuestAccessible(pagePath: string): boolean {
  return GUEST_ACCESSIBLE_PAGES.includes(pagePath)
}
