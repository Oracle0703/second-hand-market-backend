// @vitest-environment jsdom

import React from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { act } from 'react-dom/test-utils'
import { afterEach, describe, expect, test, vi } from 'vitest'
import Taro from '@tarojs/taro'

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

vi.mock('@tarojs/taro', () => ({
  default: {
    getSystemInfoSync: vi.fn(() => ({ platform: 'devtools' })),
    makePhoneCall: vi.fn(),
    navigateTo: vi.fn(),
    openLocation: vi.fn(),
    showModal: vi.fn(),
    showToast: vi.fn()
  },
  useDidShow: vi.fn(),
  useReachBottom: vi.fn()
}))

vi.mock('@tarojs/components', async () => {
  const react = await import('react')
  const component = (tag: string) => ({
    children,
    autoplay: _autoplay,
    circular: _circular,
    duration: _duration,
    indicatorDots: _indicatorDots,
    interval: _interval,
    lazyLoad: _lazyLoad,
    mode: _mode,
    size: _size,
    ...props
  }: {
    children?: React.ReactNode
    autoplay?: boolean
    circular?: boolean
    duration?: number
    indicatorDots?: boolean
    interval?: number
    lazyLoad?: boolean
    mode?: string
    size?: string
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
  fetchBuyerProducts: vi.fn(async () => ({
    items: [],
    total: 0,
    page: 1,
    page_size: 10
  }))
}))

import HomePage from '../src/pages/home/index'

let container: HTMLDivElement
let root: Root

afterEach(() => {
  act(() => {
    root.unmount()
  })
  container.remove()
})

describe('首页视频导航入口', () => {
  test('配置视频时展示视频入口并跳转到播放页', () => {
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)

    act(() => {
      root.render(React.createElement(HomePage))
    })

    const labels = Array.from(container.querySelectorAll('button'), (button) => button.textContent)
    expect(labels).toContain('拨打电话')
    expect(labels).toContain('视频导航')
    expect(labels).toContain('导航去店')

    const videoButton = Array.from(container.querySelectorAll('button')).find((button) => button.textContent === '视频导航')
    expect(videoButton).toBeTruthy()

    act(() => {
      videoButton?.click()
    })

    expect(Taro.navigateTo).toHaveBeenCalledWith({ url: '/pages/store-guide/index' })
  })
})
