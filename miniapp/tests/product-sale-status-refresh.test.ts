// @vitest-environment jsdom

import React from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { act } from 'react-dom/test-utils'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '../src/libs/react-query'

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

const runtimeState = vi.hoisted(() => ({
  detailStatus: 'ON_SHELF',
  favoriteStatus: 'ON_SHELF',
  didShowCallback: null as (() => void) | null
}))

vi.mock('@tarojs/taro', () => ({
  default: {
    navigateTo: vi.fn(),
    previewImage: vi.fn()
  },
  useDidShow: (callback: () => void) => {
    runtimeState.didShowCallback = callback
  },
  useRouter: () => ({ params: { id: '1' } }),
  useShareAppMessage: vi.fn()
}))

vi.mock('@tarojs/components', async () => {
  const react = await import('react')
  const component = (tag: string) => ({
    children,
    loading: _loading,
    size: _size,
    mode: _mode,
    indicatorDots: _indicatorDots,
    circular: _circular,
    current: _current,
    onChange: _onChange,
    ...props
  }: {
    children?: React.ReactNode
    loading?: boolean
    size?: string
    mode?: string
    indicatorDots?: boolean
    circular?: boolean
    current?: number
    onChange?: unknown
  }) => react.createElement(tag, props, children)

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
  addFavorite: vi.fn(async () => ({ product_id: 1, is_favorited: true })),
  fetchBuyerProductDetail: vi.fn(async () => ({
    product: {
      id: 1,
      title: '状态刷新商品',
      description: '商品描述',
      price_cent: 1200,
      original_price_cent: 1500,
      stock: runtimeState.detailStatus === 'SOLD' ? 0 : 2,
      condition_level: 'GOOD',
      cover_url: '',
      status: runtimeState.detailStatus,
      merchant_id: 1,
      merchant_name: '商家',
      is_favorited: false,
      can_submit_intent: runtimeState.detailStatus === 'ON_SHELF',
      images: [],
      merchant: { id: 1, name: '商家' }
    }
  })),
  listFavorites: vi.fn(async () => ({
    items: [{
      product_id: 1,
      title: '收藏商品',
      cover_url: '',
      price_cent: 1200,
      original_price_cent: 1500,
      stock: runtimeState.favoriteStatus === 'SOLD' ? 0 : 2,
      status: runtimeState.favoriteStatus,
      favorited_at: '2026-08-10T00:00:00Z'
    }],
    total: 1,
    page: 1,
    page_size: 50
  })),
  removeFavorite: vi.fn(async () => ({ product_id: 1, is_favorited: false })),
  reportView: vi.fn(async () => ({
    product_id: 1,
    last_viewed_at: '2026-08-10T00:00:00Z',
    view_count: 1
  }))
}))

import FavoritePage from '../src/pages/favorite/index'
import ProductDetailPage from '../src/pages/product/detail/index'
import { fetchBuyerProductDetail, listFavorites } from '../src/services/buyer'

let container: HTMLDivElement
let root: Root

async function renderPage(page: React.ReactElement) {
  container = document.createElement('div')
  document.body.appendChild(container)
  root = createRoot(container)
  const queryClient = new QueryClient()

  await act(async () => {
    root.render(
      React.createElement(QueryClientProvider, { client: queryClient }, page)
    )
    await new Promise((resolve) => setTimeout(resolve, 0))
  })
}

async function triggerDidShow() {
  const callback = runtimeState.didShowCallback
  if (!callback) {
    throw new Error('useDidShow callback is not registered')
  }

  await act(async () => {
    callback()
    await new Promise((resolve) => setTimeout(resolve, 0))
  })
}

function buttonLabels() {
  return Array.from(container.querySelectorAll('button'), (button) => button.textContent)
}

describe('商品售卖状态恢复显示刷新', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    runtimeState.detailStatus = 'ON_SHELF'
    runtimeState.favoriteStatus = 'ON_SHELF'
    runtimeState.didShowCallback = null
  })

  afterEach(() => {
    act(() => {
      root.unmount()
    })
    container.remove()
  })

  test('详情页重新显示后刷新售罄状态并移除联系入口', async () => {
    await renderPage(React.createElement(ProductDetailPage))

    await vi.waitFor(() => {
      expect(fetchBuyerProductDetail).toHaveBeenCalledTimes(1)
      expect(container.textContent).toContain('在售')
      expect(buttonLabels()).toContain('我想要')
    })

    await triggerDidShow()
    expect(fetchBuyerProductDetail).toHaveBeenCalledTimes(1)

    runtimeState.detailStatus = 'SOLD'
    await triggerDidShow()

    await vi.waitFor(() => {
      expect(fetchBuyerProductDetail).toHaveBeenCalledTimes(2)
      expect(container.textContent).toContain('售罄')
      expect(buttonLabels()).not.toContain('我想要')
    })
  })

  test('收藏页重新显示后刷新售罄状态并移除联系入口', async () => {
    await renderPage(React.createElement(FavoritePage))

    await vi.waitFor(() => {
      expect(listFavorites).toHaveBeenCalledTimes(1)
      expect(container.textContent).toContain('在售')
      expect(buttonLabels()).toContain('我想要')
    })

    await triggerDidShow()
    expect(listFavorites).toHaveBeenCalledTimes(1)

    runtimeState.favoriteStatus = 'SOLD'
    await triggerDidShow()

    await vi.waitFor(() => {
      expect(listFavorites).toHaveBeenCalledTimes(2)
      expect(container.textContent).toContain('售罄')
      expect(buttonLabels()).not.toContain('我想要')
    })
  })

  test('详情页状态刷新失败时展示错误且不产生未处理拒绝', async () => {
    await renderPage(React.createElement(ProductDetailPage))
    await vi.waitFor(() => expect(fetchBuyerProductDetail).toHaveBeenCalledTimes(1))
    await triggerDidShow()

    vi.mocked(fetchBuyerProductDetail).mockRejectedValueOnce(new Error('详情状态刷新失败'))
    await triggerDidShow()

    await vi.waitFor(() => {
      expect(container.textContent).toContain('详情状态刷新失败')
      expect(container.textContent).toContain('在售')
    })
  })

  test('收藏页状态刷新失败时展示错误且不产生未处理拒绝', async () => {
    await renderPage(React.createElement(FavoritePage))
    await vi.waitFor(() => expect(listFavorites).toHaveBeenCalledTimes(1))
    await triggerDidShow()

    vi.mocked(listFavorites).mockRejectedValueOnce(new Error('收藏状态刷新失败'))
    await triggerDidShow()

    await vi.waitFor(() => {
      expect(container.textContent).toContain('收藏状态刷新失败')
      expect(container.textContent).toContain('收藏商品')
    })
  })
})
