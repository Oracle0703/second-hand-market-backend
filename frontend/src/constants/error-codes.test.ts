import { describe, expect, it } from 'vitest'
import { ERROR_MESSAGES, apiErrorMessage } from './error-codes'

describe('error code map', () => {
  it('contains key business errors', () => {
    expect(ERROR_MESSAGES[10005]).toBe('当前状态下不可执行该操作')
    expect(ERROR_MESSAGES[10006]).toBe('当前账号处于入驻受限状态，仅可访问入驻流程功能')
    expect(ERROR_MESSAGES[10010]).toBe('商品已被其他订单占用')
  })

  it('uses actionable image upload messages for upload endpoints', () => {
    expect(apiErrorMessage(10001, '/files/presign')).toBe('图片文件信息异常，请重新选择图片后再上传')
    expect(apiErrorMessage(10001, '/files/upload')).toBe('图片上传参数已失效，请重新选择图片后再上传')
    expect(apiErrorMessage(10008, '/files/upload')).toBe('图片处理失败，请换一张图片，或先降低分辨率/压缩后再上传')
  })

  it('keeps generic messages for non-upload endpoints', () => {
    expect(apiErrorMessage(10001, '/merchant/products')).toBe('参数校验失败')
  })
})
