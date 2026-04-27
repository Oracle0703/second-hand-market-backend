import { describe, expect, test } from 'vitest'
import packageJSON from '../package.json'

describe('小程序运行时依赖兼容性', () => {
  test('react 与 react-dom 版本应与 Taro 3.6 运行时保持兼容', () => {
    expect(packageJSON.dependencies.react).toMatch(/\^?18\.2\.0$/)
    expect(packageJSON.dependencies['react-dom']).toMatch(/\^?18\.2\.0$/)
  })
})
