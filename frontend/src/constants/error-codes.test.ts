import { describe, expect, it } from 'vitest'
import { ERROR_MESSAGES } from './error-codes'

describe('error code map', () => {
  it('contains key business errors', () => {
    expect(ERROR_MESSAGES[10005]).toBe('当前状态下不可执行该操作')
    expect(ERROR_MESSAGES[10006]).toBe('当前账号处于入驻受限状态，仅可访问入驻流程功能')
    expect(ERROR_MESSAGES[10010]).toBe('商品已被其他订单占用')
  })
})
