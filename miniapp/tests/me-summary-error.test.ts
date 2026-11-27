// @vitest-environment jsdom

import React from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { act } from 'react-dom/test-utils'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

const mocks = vi.hoisted(() => ({
  didShow: [] as Array<() => void>,
  fetchSummary: vi.fn(),
  logoutBuyer: vi.fn(),
  clearSession: vi.fn()
}))

vi.mock('@tarojs/taro', () => ({
  default: { navigateTo: vi.fn() },
  useDidShow: (callback: () => void) => {
    mocks.didShow.push(callback)
  }
}))

vi.mock('@tarojs/components', async () => {
  const react = await import('react')
  const component = (tag: string) => ({ children, ...props }: { children?: React.ReactNode }) => react.createElement(tag, props, children)
  return { Button: component('button'), Text: component('span'), View: component('div') }
})

vi.mock('../src/services/buyer', () => ({
  fetchSummary: mocks.fetchSummary,
  logoutBuyer: mocks.logoutBuyer
}))

vi.mock('../src/stores/session', () => ({
  useSessionStore: (selector: (state: { clearSession: () => void }) => unknown) => selector({ clearSession: mocks.clearSession })
}))

vi.mock('../src/utils/platform', () => ({
  getMiniProgramPlatform: () => ({ defaultNickname: '微信用户' })
}))

import { QueryClient, QueryClientProvider } from '../src/libs/react-query'
import MePage from '../src/pages/me/index'

let container: HTMLDivElement
let root: Root
const unhandled = vi.fn()

beforeEach(() => {
  mocks.didShow.length = 0
  mocks.fetchSummary.mockReset()
  mocks.logoutBuyer.mockReset().mockResolvedValue(undefined)
  mocks.clearSession.mockReset()
  unhandled.mockReset()
  process.on('unhandledRejection', unhandled)
  container = document.createElement('div')
  document.body.appendChild(container)
  root = createRoot(container)
})

afterEach(async () => {
  await act(async () => root.unmount())
  container.remove()
  process.off('unhandledRejection', unhandled)
})

async function renderPage() {
  await act(async () => {
    root.render(React.createElement(QueryClientProvider, { client: new QueryClient() }, React.createElement(MePage)))
  })
}

async function flushUnhandled() {
  await new Promise((resolve) => setTimeout(resolve, 0))
}

function buttons() {
  return Array.from(container.querySelectorAll('button')).map((button) => button.textContent)
}

describe('我的页 summary 查询失败', () => {
  test('失败时显示错误与重试，不误显示为游客模式，也不产生未处理的 rejection', async () => {
    mocks.fetchSummary.mockRejectedValue(new Error('网络异常'))
    await renderPage()
    await act(async () => mocks.didShow.at(-1)?.())
    await flushUnhandled()

    expect(container.textContent).toContain('账户信息加载失败')
    expect(container.textContent).toContain('网络异常')
    expect(container.textContent).not.toContain('当前为游客模式')
    expect(container.textContent).toContain('收藏: --')
    expect(buttons()).toEqual(['重试'])
    expect(unhandled).not.toHaveBeenCalled()

    mocks.fetchSummary.mockResolvedValue({
      is_login: true,
      profile: { nickname: '买家A' },
      counters: { favorites: 2, histories: 3, intents_open: 0 }
    })
    await act(async () => container.querySelector('button')?.click())

    expect(container.textContent).toContain('买家A')
    expect(container.textContent).toContain('收藏: 2')
    expect(container.textContent).not.toContain('网络异常')
    expect(buttons()).toEqual(['退出登录'])
  })

  test('退出登录后刷新失败不会产生未处理的 rejection', async () => {
    mocks.fetchSummary.mockResolvedValueOnce({
      is_login: true,
      profile: { nickname: '买家A' },
      counters: { favorites: 1, histories: 0, intents_open: 0 }
    })
    await renderPage()
    mocks.fetchSummary.mockRejectedValue(new Error('网络异常'))

    await act(async () => container.querySelector('button')?.click())
    await flushUnhandled()

    expect(mocks.clearSession).toHaveBeenCalled()
    expect(container.textContent).toContain('网络异常')
    expect(unhandled).not.toHaveBeenCalled()
  })
})
