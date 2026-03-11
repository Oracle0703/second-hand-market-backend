import { describe, expect, test } from 'vitest'
import appConfig from '../src/app.config'
import { isGuestAccessible } from '../src/utils/navigation'

describe('游客访问能力', () => {
  test('首页/列表/详情对游客开放', () => {
    expect(appConfig.pages).toContain('pages/home/index')
    expect(appConfig.pages).toContain('pages/product/list/index')
    expect(appConfig.pages).toContain('pages/product/detail/index')
    expect(isGuestAccessible('pages/home/index')).toBe(true)
    expect(isGuestAccessible('pages/product/list/index')).toBe(true)
    expect(isGuestAccessible('pages/product/detail/index')).toBe(true)
  })
})
