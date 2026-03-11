import { describe, expect, test } from 'vitest'
import { nextFavoriteState } from '../src/utils/favorites'

describe('收藏交互', () => {
  test('收藏与取消收藏状态切换正确', () => {
    expect(nextFavoriteState(false, 'add')).toBe(true)
    expect(nextFavoriteState(true, 'remove')).toBe(false)
  })
})
