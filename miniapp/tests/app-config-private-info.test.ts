import { describe, expect, test } from 'vitest'
import appConfig from '../src/app.config'

const allowedPrivateInfos = new Set([
  'chooseAddress',
  'chooseLocation',
  'choosePoi',
  'getFuzzyLocation',
  'getLocation',
  'onLocationChange',
  'startLocationUpdate',
  'startLocationUpdateBackground'
])

describe('小程序隐私接口配置', () => {
  test('requiredPrivateInfos 使用微信要求的接口名', () => {
    const requiredPrivateInfos = appConfig.requiredPrivateInfos ?? []

    for (const info of requiredPrivateInfos) {
      expect(allowedPrivateInfos.has(info)).toBe(true)
    }
  })
})
