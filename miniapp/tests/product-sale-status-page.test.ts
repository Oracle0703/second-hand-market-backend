// @vitest-environment jsdom

import React from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { act } from 'react-dom/test-utils'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

const pageState = vi.hoisted(() => ({
  detailProduct: null as Record<string, unknown> | null,
  favorites: [] as Array<Record<string, unknown>>
}))

vi.mock('@tarojs/taro', () => ({
  default: {
    navigateTo: vi.fn(),
    previewImage: vi.fn()
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

vi.mock('@/libs/react-query', () => ({
  useMutation: () => ({ isPending: false, mutate: vi.fn() }),
  useQuery: ({ queryKey }: { queryKey: unknown[] }) => {
    if (queryKey[0] === 'buyer-product-detail') {
      return { data: pageState.detailProduct ? { product: pageState.detailProduct } : undefined, error: null, isLoading: false }
    }
    if (queryKey[0] === 'buyer-favorites') {
      return { data: { items: pageState.favorites }, error: null, isLoading: false }
    }
    return { data: undefined, error: null, isLoading: false }
  },
  useQueryClient: () => ({ invalidateQueries: vi.fn() })
}))

import FavoritePage from '../src/pages/favorite/index'
import ProductDetailPage from '../src/pages/product/detail/index'

let container: HTMLDivElement
let root: Root

function renderPage(page: React.ReactElement) {
  container = document.createElement('div')
  document.body.appendChild(container)
  root = createRoot(container)
  act(() => {
    root.render(page)
  })
  return container
}

function renderDetail(status: string, canSubmitIntent = true) {
  pageState.detailProduct = {
    id: 1,
    title: '页面商品',
    description: '商品描述',
    price_cent: 1200,
    stock: status === 'SOLD' ? 0 : 2,
    condition_level: 'GOOD',
    cover_url: '',
    status,
    merchant_id: 1,
    merchant_name: '商家',
    is_favorited: false,
    can_submit_intent: canSubmitIntent,
    images: [],
    merchant: { id: 1, name: '商家' }
  }
  return renderPage(React.createElement(ProductDetailPage))
}

function renderFavorite(status: string) {
  pageState.favorites = [
    {
      product_id: 1,
      title: '收藏商品',
      cover_url: '',
      price_cent: 1200,
      stock: status === 'SOLD' ? 0 : 2,
      status,
      favorited_at: '2026-08-10T00:00:00Z'
    }
  ]
  return renderPage(React.createElement(FavoritePage))
}

function buttonLabels(page: HTMLElement) {
  return Array.from(page.querySelectorAll('button'), (button) => button.textContent)
}

describe('小程序商品售卖状态页面', () => {
  beforeEach(() => {
    pageState.detailProduct = null
    pageState.favorites = []
  })

  afterEach(() => {
    act(() => {
      root.unmount()
    })
    container.remove()
  })

  test('详情页对 SOLD 显示售罄且不渲染我想要', () => {
    const page = renderDetail('SOLD')

    expect(page.querySelector('.status-badge')?.textContent).toBe('售罄')
    expect(buttonLabels(page)).toEqual(['收藏'])
  })

  test('详情页对在售且允许提交意向的商品渲染我想要', () => {
    const page = renderDetail('ON_SHELF')

    expect(page.querySelector('.status-badge')?.textContent).toBe('在售')
    expect(buttonLabels(page)).toEqual(['收藏', '我想要'])
  })

  test('详情页在 can_submit_intent=false 时隐藏在售商品的我想要入口', () => {
    const page = renderDetail('ON_SHELF', false)

    expect(buttonLabels(page)).toEqual(['收藏'])
  })

  test('收藏页对 SOLD 显示售罄且不渲染我想要', () => {
    const page = renderFavorite('SOLD')

    expect(page.querySelector('.status-badge')?.textContent).toBe('售罄')
    expect(buttonLabels(page)).toEqual(['取消收藏'])
  })

  test('收藏页对在售商品渲染我想要', () => {
    const page = renderFavorite('ON_SHELF')

    expect(page.querySelector('.status-badge')?.textContent).toBe('在售')
    expect(buttonLabels(page)).toEqual(['取消收藏', '我想要'])
  })
})
