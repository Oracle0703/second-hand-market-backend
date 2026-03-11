import { describe, expect, test } from 'vitest'
import { toBuyerStatusText } from '../src/utils/intent'

describe('我的意向状态展示', () => {
  test('状态映射符合文档', () => {
    expect(toBuyerStatusText('NEW')).toBe('处理中')
    expect(toBuyerStatusText('CONTACTED')).toBe('已联系')
    expect(toBuyerStatusText('CLOSED')).toBe('已关闭')
  })
})
