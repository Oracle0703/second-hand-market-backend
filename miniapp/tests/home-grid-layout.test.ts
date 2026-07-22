import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, test } from 'vitest'

const appStyles = readFileSync(resolve(__dirname, '../src/styles/app.scss'), 'utf8')

function ruleBody(selector: string): string {
  const escapedSelector = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const match = appStyles.match(new RegExp(`${escapedSelector}\\s*\\{([^}]*)\\}`, 's'))

  expect(match, `${selector} 样式不存在`).not.toBeNull()
  return match?.[1] ?? ''
}

describe('首页商品双列布局', () => {
  test('首页商品卡片使用稳定的双列规则，避免小屏塌成单列', () => {
    const gridRule = ruleBody('.home-grid')
    const cardRule = ruleBody('.home-product-card')

    expect(gridRule).toContain('display: flex;')
    expect(gridRule).toContain('width: 100%;')
    expect(gridRule).toContain('box-sizing: border-box;')
    expect(gridRule).toContain('justify-content: space-between;')
    expect(gridRule).not.toMatch(/\bgap:/)

    expect(cardRule).toContain('flex: 0 0 48.5%;')
    expect(cardRule).toContain('width: 48.5%;')
    expect(cardRule).toContain('max-width: 48.5%;')
    expect(cardRule).toContain('min-width: 0;')
    expect(cardRule).not.toContain('calc(')
  })
})
