import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, test } from 'vitest'

const appStyles = readFileSync(resolve(__dirname, '../src/styles/app.scss'), 'utf8')

describe('首页商品双列布局', () => {
  test('首页商品卡片使用稳定的双列规则，避免小屏塌成单列', () => {
    expect(appStyles).toContain('.home-grid {')
    expect(appStyles).toContain('justify-content: space-between;')
    expect(appStyles).toContain('.home-product-card {')
    expect(appStyles).toContain('width: 48.5%;')
    expect(appStyles).toContain('min-width: 0;')
    expect(appStyles).not.toContain('width: calc(50% - 9rpx);')
  })
})
