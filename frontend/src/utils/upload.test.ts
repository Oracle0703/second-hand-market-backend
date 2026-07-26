import { describe, expect, it } from 'vitest'
import { validateUploadFile } from './upload'

const tenMiB = 10 * 1024 * 1024

describe('validateUploadFile', () => {
  it('accepts exactly 10 MiB and rejects one byte more', () => {
    const exact = new File([new Uint8Array(tenMiB)], 'exact.jpg', { type: 'image/jpeg' })
    const over = new File([new Uint8Array(tenMiB + 1)], 'over.jpg', { type: 'image/jpeg' })

    expect(validateUploadFile(exact)).toBeNull()
    expect(validateUploadFile(over)).toBe('图片不能超过 10 MiB')
  })

  it('rejects an empty file', () => {
    expect(validateUploadFile(new File([], 'empty.jpg', { type: 'image/jpeg' }))).toBe('图片文件不能为空')
  })
})
