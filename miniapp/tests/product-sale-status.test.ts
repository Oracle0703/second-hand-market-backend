import { describe, expect, test } from 'vitest'
import { canContactForProduct, getProductStatusText } from '../src/utils/product-status'

describe('商品售卖状态', () => {
  test.each([
    ['DRAFT', '草稿'],
    ['ON_SHELF', '在售'],
    ['LOCKED', '锁定'],
    ['OFF_SHELF', '下架'],
    ['SOLD', '售罄'],
    ['CUSTOM', 'CUSTOM']
  ])('商品状态显示对应文案：%s', (status, expected) => {
    expect(getProductStatusText(status)).toBe(expected)
  })

  test.each([
    ['DRAFT', false],
    ['ON_SHELF', true],
    ['LOCKED', false],
    ['OFF_SHELF', false],
    ['SOLD', false]
  ])('只有在售商品可以联系购买：%s', (status, expected) => {
    expect(canContactForProduct(status)).toBe(expected)
  })

  test('详情明确禁止提交意向时隐藏联系购买入口', () => {
    expect(canContactForProduct('ON_SHELF', false)).toBe(false)
  })
})
