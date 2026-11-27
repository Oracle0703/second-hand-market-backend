// @vitest-environment jsdom
import React from 'react'
import { createRoot } from 'react-dom/client'
import { act } from 'react-dom/test-utils'
import { beforeEach, describe, expect, test, vi } from 'vitest'

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

const mocks = vi.hoisted(() => ({
  loginByMiniProgram: vi.fn(),
  mergeGuest: vi.fn(),
  showToast: vi.fn(),
  redirectTo: vi.fn()
}))
vi.mock('@tarojs/taro', () => ({
  default: {
    login: async () => ({ code: 'test-code' }),
    getStorageSync: () => undefined,
    setStorageSync: vi.fn(),
    removeStorageSync: vi.fn(),
    showToast: mocks.showToast,
    redirectTo: mocks.redirectTo
  },
  useDidShow: vi.fn(),
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
vi.mock('../src/utils/platform', () => ({
  getMiniProgramPlatform: () => ({ provider: 'wechat', loginTitle: '登录', defaultNickname: '买家' })
}))
vi.mock('../src/services/buyer', () => ({
  loginByMiniProgram: mocks.loginByMiniProgram,
  mergeGuest: mocks.mergeGuest
}))

import LoginPage from '../src/pages/login/index'
import { useSessionStore } from '../src/stores/session'

beforeEach(() => {
  vi.resetAllMocks()
  useSessionStore.setState(useSessionStore.getInitialState(), true)
  useSessionStore.getState().setDeviceID('test-device')
})

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
    expect(useSessionStore.getState().profile?.id).toBe(1)
    expect(mocks.mergeGuest).toHaveBeenCalledWith('test-device')
    expect(mocks.showToast).toHaveBeenCalledWith({ title: '游客数据同步失败', icon: 'none' })
    expect(mocks.redirectTo).toHaveBeenCalledWith({ url: '/pages/history/index' })
    expect(content).not.toContain('同步暂不可用')
  })

  test('平台登录失败时保留登录页及错误信息', async () => {
    mocks.loginByMiniProgram.mockRejectedValue(new Error('登录不可用'))
    expect(await submitLogin()).toContain('登录不可用')
    expect(useSessionStore.getState().accessToken).toBe('')
    expect(mocks.mergeGuest).not.toHaveBeenCalled()
    expect(mocks.redirectTo).not.toHaveBeenCalled()
  })

  test('合并过程中登录失效时停留在登录页', async () => {
    mocks.loginByMiniProgram.mockResolvedValue({ access_token: 'access', refresh_token: 'refresh', user: { id: 1 } })
    mocks.mergeGuest.mockImplementationOnce(async () => {
      useSessionStore.getState().clearSession()
      throw new Error('登录已过期')
    })
    expect(await submitLogin()).toContain('登录已过期，请重新登录')
    expect(mocks.showToast).not.toHaveBeenCalled()
    expect(mocks.redirectTo).not.toHaveBeenCalled()
  })

  test.each([true, false])('另一买家登录后旧合并结果不清会话也不跳转，合并失败=%s', async (failed) => {
    mocks.loginByMiniProgram.mockResolvedValue({ access_token: 'access-A', refresh_token: 'refresh-A', user: { id: 1 } })
    mocks.mergeGuest.mockImplementationOnce(async () => {
      useSessionStore.getState().setSession('access-B', 'refresh-B', { id: 2, buyer_no: 'B' })
      if (failed) throw new Error('旧合并失败')
    })
    await submitLogin()
    expect(useSessionStore.getState().accessToken).toBe('access-B')
    expect(mocks.showToast).not.toHaveBeenCalled()
    expect(mocks.redirectTo).not.toHaveBeenCalled()
  })

  test('合并期间同一会话正常续签仍继续跳转', async () => {
    mocks.loginByMiniProgram.mockResolvedValue({ access_token: 'access', refresh_token: 'refresh', user: { id: 1 } })
    mocks.mergeGuest.mockImplementationOnce(async () => useSessionStore.getState().rotateTokens('next-access', 'next-refresh'))
    await submitLogin()
    expect(useSessionStore.getState().accessToken).toBe('next-access')
    expect(mocks.redirectTo).toHaveBeenCalledWith({ url: '/pages/history/index' })
  })
})
