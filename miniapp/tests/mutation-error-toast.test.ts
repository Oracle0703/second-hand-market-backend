// @vitest-environment jsdom

import React from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { act } from 'react-dom/test-utils'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

const mocks = vi.hoisted(() => ({
  showToast: vi.fn(),
  addFavorite: vi.fn(),
  removeFavorite: vi.fn(),
  clearHistories: vi.fn(),
  listFavorites: vi.fn(),
  listHistories: vi.fn(),
  fetchBuyerProductDetail: vi.fn(),
  reportView: vi.fn()
}))

vi.mock('@tarojs/taro', () => ({
  default: {
    navigateTo: vi.fn(),
    previewImage: vi.fn(),
    showToast: mocks.showToast
  },
  useDidShow: vi.fn(),
  useRouter: () => ({ params: { id: '1' } }),
  useShareAppMessage: vi.fn()
}))

vi.mock('@tarojs/components', async () => {
  const react = await import('react')
  const component = (tag: string) => ({ children, loading: _loading, size: _size, mode: _mode, indicatorDots: _indicatorDots, circular: _circular, current: _current, onChange: _onChange, ...props }: { children?: React.ReactNode; loading?: boolean; size?: string; mode?: string; indicatorDots?: boolean; circular?: boolean; current?: number; onChange?: unknown }) =>
    react.createElement(tag, props, children)

  return {
    Button: component('button'),
    Image: component('img'),
    Swiper: component('swiper'),
    SwiperItem: component('swiper-item'),
    Text: component('span'),
    View: component('div')
  }
})

vi.mock('../src/services/buyer', () => ({
  addFavorite: mocks.addFavorite,
  removeFavorite: mocks.removeFavorite,
  clearHistories: mocks.clearHistories,
  listFavorites: mocks.listFavorites,
  listHistories: mocks.listHistories,
  fetchBuyerProductDetail: mocks.fetchBuyerProductDetail,
  reportView: mocks.reportView
}))

vi.mock('../src/utils/contact', () => ({ promptAndCallStore: vi.fn() }))

import { QueryClient, QueryClientProvider } from '../src/libs/react-query'
import { getMutationErrorMessage } from '../src/hooks/useToastMutation'
import { AuthExpiredError } from '../src/services/request'
import FavoritePage from '../src/pages/favorite/index'
import HistoryPage from '../src/pages/history/index'
import ProductDetailPage from '../src/pages/product/detail/index'

const productItem = {
  product_id: 1,
  title: '商品',
  cover_url: '',
  price_cent: 1200,
  stock: 2,
  status: 'ON_SHELF',
  view_count: 1
}

let container: HTMLDivElement
let root: Root

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

async function renderPage(page: React.FC) {
  container = document.createElement('div')
  document.body.appendChild(container)
  root = createRoot(container)
  await act(async () => {
    root.render(React.createElement(QueryClientProvider, { client: new QueryClient(), children: React.createElement(page) }))
  })
}

function findButton(label: string): HTMLButtonElement {
  const button = Array.from(container.querySelectorAll('button')).find((item) => item.textContent === label)
  if (!button) throw new Error(`button ${label} not found`)
  return button
}

async function expectPendingThenToast(label: string, service: ReturnType<typeof vi.fn>, toastTitle: string) {
  const pending = deferred<void>()
  service.mockReturnValueOnce(pending.promise)

  const button = findButton(label)
  await act(async () => {
    button.click()
    button.click()
  })

  expect(service).toHaveBeenCalledTimes(1)
  expect(findButton(label).disabled).toBe(true)

  await act(async () => {
    pending.reject(new Error('request failed: status=500, url=/x, payload=null'))
  })

  expect(mocks.showToast).toHaveBeenCalledWith({ title: toastTitle, icon: 'none' })
  expect(findButton(label).disabled).toBe(false)
}

describe('小程序 mutation 失败提示与防重复提交', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.listFavorites.mockResolvedValue({ items: [productItem], total: 1 })
    mocks.listHistories.mockResolvedValue({ items: [productItem], total: 1 })
    mocks.fetchBuyerProductDetail.mockResolvedValue({
      product: { id: 1, title: '商品', description: '', price_cent: 1200, stock: 2, status: 'ON_SHELF', is_favorited: false, can_submit_intent: true, images: [] }
    })
    mocks.reportView.mockResolvedValue(undefined)
  })

  afterEach(async () => {
    await act(async () => {
      root.unmount()
    })
    container.remove()
  })

  test('收藏页取消收藏失败时提示，且请求中按钮禁用', async () => {
    await renderPage(FavoritePage)
    await expectPendingThenToast('取消收藏', mocks.removeFavorite, '取消收藏失败，请重试')
  })

  test('浏览记录页清空失败时提示，且请求中按钮禁用', async () => {
    await renderPage(HistoryPage)
    await expectPendingThenToast('清空', mocks.clearHistories, '清空浏览记录失败，请重试')
  })

  test('详情页收藏失败时提示，且请求中按钮禁用', async () => {
    await renderPage(ProductDetailPage)
    await expectPendingThenToast('收藏', mocks.addFavorite, '收藏操作失败，请重试')
  })

  test('详情页登录过期时提示过期原因', async () => {
    mocks.addFavorite.mockRejectedValueOnce(new AuthExpiredError())
    await renderPage(ProductDetailPage)
    await act(async () => {
      findButton('收藏').click()
    })
    expect(mocks.showToast).toHaveBeenCalledWith({ title: '登录已过期，请重新登录', icon: 'none' })
  })
})

describe('getMutationErrorMessage', () => {
  test('业务错误展示原因，技术错误或非 Error 使用兜底文案', () => {
    expect(getMutationErrorMessage(new Error('商品已下架'), '兜底')).toBe('商品已下架')
    expect(getMutationErrorMessage(new Error('request failed: status=500, url=/x, payload=null'), '兜底')).toBe('兜底')
    expect(getMutationErrorMessage(new Error('API response malformed: status=200'), '兜底')).toBe('兜底')
    expect(getMutationErrorMessage('oops', '兜底')).toBe('兜底')
  })
})
