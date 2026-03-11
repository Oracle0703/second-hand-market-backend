import { describe, expect, test } from 'vitest'
import { buildLoginRedirect, canSubmitIntentWhenLoggedIn, resolveLoginRedirect } from '../src/utils/login'

describe('登录跳转与状态', () => {
  test('登录后回跳地址构建和解析正确', () => {
    const target = '/pages/intent/create/index?product_id=12'
    const loginURL = buildLoginRedirect(target)
    expect(loginURL.includes('/pages/login/index?redirect=')).toBe(true)
    const encoded = loginURL.split('redirect=')[1]
    expect(resolveLoginRedirect(encoded)).toBe(target)
  })

  test('登录态决定是否允许提交意向', () => {
    expect(canSubmitIntentWhenLoggedIn(false)).toBe(false)
    expect(canSubmitIntentWhenLoggedIn(true)).toBe(true)
  })
})
