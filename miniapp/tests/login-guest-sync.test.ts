// @vitest-environment jsdom
import React from 'react'
import { createRoot } from 'react-dom/client'
import { act } from 'react-dom/test-utils'
import { afterEach, describe, expect, test, vi } from 'vitest'

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

const mocks = vi.hoisted(() => ({
  loginByMiniProgram: vi.fn(),
  mergeGuest: vi.fn(),
  setSession: vi.fn(),
  showToast: vi.fn(),
  redirectTo: vi.fn()
}))
vi.mock('@tarojs/taro', () => ({
  default: {
    login: async () => ({ code: 'test-code' }),
    showToast: mocks.showToast,
    redirectTo: mocks.redirectTo
  },
  useRouter: () => ({ params: { redirect: '%2Fpages%2Fhistory%2Findex' } })
}))
vi.mock('@tarojs/components', async () => {
  const { createElement } = await import('react')
  return {
    View: 'div',
    Text: 'span',
    Button: ({ children, onClick }: React.ButtonHTMLAttributes<HTMLButtonElement>) =>
      createElement('button', { onClick }, children)
  }
})
vi.mock('../src/stores/session', () => ({
  ensureDeviceID: () => 'test-device',
  useSessionStore: { getState: () => ({ setSession: mocks.setSession }) }
}))
vi.mock('../src/utils/platform', () => ({
  getMiniProgramPlatform: () => ({ provider: 'wechat', loginTitle: '登录', defaultNickname: '买家' })
}))
vi.mock('../src/services/buyer', () => ({
  loginByMiniProgram: mocks.loginByMiniProgram,
  mergeGuest: mocks.mergeGuest
}))

import LoginPage from '../src/pages/login/index'

afterEach(() => vi.resetAllMocks())

async function submitLogin() {
  const container = document.createElement('div')
  const root = createRoot(container)
  try {
    await act(async () => root.render(React.createElement(LoginPage)))
    await act(async () => container.querySelector('button')!.click())
    return container.textContent
  } finally {
    await act(async () => root.unmount())
  }
}

describe('游客合并与登录结果', () => {
  test('已登录时合并失败仍跳回目标页，并单独提示同步失败', async () => {
    mocks.loginByMiniProgram.mockResolvedValue({ access_token: 'access', refresh_token: 'refresh', user: { id: 1 } })
    mocks.mergeGuest.mockRejectedValue(new Error('同步暂不可用'))
    const content = await submitLogin()
    expect(mocks.setSession).toHaveBeenCalledWith('access', 'refresh', { id: 1 })
    expect(mocks.mergeGuest).toHaveBeenCalledWith('test-device')
    expect(mocks.showToast).toHaveBeenCalledWith({ title: '游客数据同步失败', icon: 'none' })
    expect(mocks.redirectTo).toHaveBeenCalledWith({ url: '/pages/history/index' })
    expect(content).not.toContain('同步暂不可用')
  })

  test('平台登录失败时保留登录页及错误信息', async () => {
    mocks.loginByMiniProgram.mockRejectedValue(new Error('登录不可用'))
    expect(await submitLogin()).toContain('登录不可用')
    expect(mocks.setSession).not.toHaveBeenCalled()
    expect(mocks.mergeGuest).not.toHaveBeenCalled()
    expect(mocks.redirectTo).not.toHaveBeenCalled()
  })
})
