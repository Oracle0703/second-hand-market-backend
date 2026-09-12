// @vitest-environment jsdom

import React from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { act } from 'react-dom/test-utils'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

type PrivacyResolution = {
  event: 'exposureAuthorization' | 'agree' | 'disagree'
  buttonId?: string
}

type PrivacyListener = (resolve: (result: PrivacyResolution) => void) => void

const privacyMocks = vi.hoisted(() => ({
  listener: undefined as PrivacyListener | undefined,
  onNeedPrivacyAuthorization: vi.fn((listener: PrivacyListener) => {
    privacyMocks.listener = listener
  }),
  offNeedPrivacyAuthorization: vi.fn(),
  openPrivacyContract: vi.fn(({ success }) => success({ errMsg: 'ok' })),
  showToast: vi.fn(async () => ({}))
}))

vi.mock('@tarojs/taro', () => ({
  default: {
    onNeedPrivacyAuthorization: privacyMocks.onNeedPrivacyAuthorization,
    offNeedPrivacyAuthorization: privacyMocks.offNeedPrivacyAuthorization,
    openPrivacyContract: privacyMocks.openPrivacyContract,
    showToast: privacyMocks.showToast
  }
}))

vi.mock('@tarojs/components', async () => {
  const react = await import('react')
  const component = (tag: string) => ({ children, ...props }: { children?: React.ReactNode }) =>
    react.createElement(tag, props, children)

  return {
    Button: component('button'),
    Text: component('span'),
    View: component('div')
  }
})

import PrivacyAuthorizationDialog from '../src/components/PrivacyAuthorizationDialog'

let container: HTMLDivElement
let root: Root

beforeEach(() => {
  privacyMocks.listener = undefined
  privacyMocks.onNeedPrivacyAuthorization.mockClear()
  privacyMocks.offNeedPrivacyAuthorization.mockClear()
  privacyMocks.openPrivacyContract.mockClear()
  privacyMocks.showToast.mockClear()
  container = document.createElement('div')
  document.body.appendChild(container)
  root = createRoot(container)
  act(() => {
    root.render(React.createElement(PrivacyAuthorizationDialog))
  })
})

afterEach(() => {
  act(() => {
    root.unmount()
  })
  container.remove()
})

describe('小程序隐私授权弹窗', () => {
  test('用户点击同意按钮后携带真实 buttonId 恢复待执行的隐私接口', () => {
    const resolve = vi.fn()

    act(() => {
      privacyMocks.listener?.(resolve)
    })

    expect(resolve).toHaveBeenCalledWith({ event: 'exposureAuthorization' })
    expect(container.textContent).toContain('隐私保护提示')

    const agreeButton = container.querySelector<HTMLButtonElement>('#privacy-authorization-agree')
    expect(agreeButton).toBeTruthy()

    act(() => {
      agreeButton?.click()
    })

    expect(resolve).toHaveBeenLastCalledWith({
      event: 'agree',
      buttonId: 'privacy-authorization-agree'
    })
    expect(container.textContent).not.toContain('隐私保护提示')
  })

  test('用户拒绝时终止待执行的隐私接口', () => {
    const resolve = vi.fn()

    act(() => {
      privacyMocks.listener?.(resolve)
    })
    const rejectButton = Array.from(container.querySelectorAll('button'))
      .find((button) => button.textContent === '暂不同意')

    act(() => {
      rejectButton?.click()
    })

    expect(resolve).toHaveBeenLastCalledWith({ event: 'disagree' })
  })
})
