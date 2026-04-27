import { describe, expect, test } from 'vitest'
import { resolveAPIBaseURL } from '../config/index'

describe('小程序 API 基址选择', () => {
  test('微信小程序开发环境默认使用正式 API 地址', () => {
    expect(
      resolveAPIBaseURL({
        taroEnv: 'weapp',
        nodeEnv: 'development'
      })
    ).toBe('https://market.meaningful.ink/api/v1')
  })

  test('显式传入 TARO_APP_API_BASE_URL 时优先使用覆盖值', () => {
    expect(
      resolveAPIBaseURL({
        taroEnv: 'weapp',
        nodeEnv: 'development',
        envBaseURL: 'https://example.com/api'
      })
    ).toBe('https://example.com/api')
  })

  test('非小程序开发环境仍可默认走本地调试地址', () => {
    expect(
      resolveAPIBaseURL({
        taroEnv: 'h5',
        nodeEnv: 'development'
      })
    ).toBe('http://localhost:8080/api/v1')
  })
})
