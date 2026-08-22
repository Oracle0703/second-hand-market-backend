import { describe, expect, it } from 'vitest'
import { MAX_IMAGE_UPLOAD_BYTES, normalizeImageMIME, validateImageFileForUpload } from './imageMime'

describe('image MIME normalization', () => {
  it.each([
    ['image/jpeg', 'photo.bin', 'image/jpeg'],
    ['image/jpg', 'photo.bin', 'image/jpeg'],
    ['image/png', 'photo.bin', 'image/png'],
    ['image/webp', 'photo.bin', 'image/webp'],
    ['image/heic', 'photo.bin', 'image/heic'],
    ['image/heif', 'photo.bin', 'image/heif']
  ])('keeps a supported browser MIME %s', (type, name, expected) => {
    expect(normalizeImageMIME({ type, name })).toBe(expected)
  })

  it.each([
    ['application/octet-stream', 'photo.heic', 'image/heic'],
    ['', 'photo.HEIF', 'image/heif'],
    ['application/octet-stream', 'photo.webp', 'image/webp'],
    ['image/svg+xml', 'photo.png', 'image/png']
  ])('falls back to the server-supported extension for %s / %s', (type, name, expected) => {
    expect(normalizeImageMIME({ type, name })).toBe(expected)
  })

  it('does not pass an unsupported browser MIME to presign', () => {
    expect(normalizeImageMIME({ type: 'text/html', name: 'poc.html' })).toBe('image/jpeg')
  })

  it('reports empty image files before upload', () => {
    expect(validateImageFileForUpload({ type: 'image/jpeg', name: 'photo.jpg', size: 0 })).toBe('图片文件为空或读取失败，请重新选择图片')
  })

  it('reports images larger than the server upload limit before upload', () => {
    expect(validateImageFileForUpload({ type: 'image/jpeg', name: 'photo.jpg', size: MAX_IMAGE_UPLOAD_BYTES + 1 })).toBe(
      '图片原图超过 40MB，请先压缩后再上传'
    )
  })

  it('reports unsupported files before upload', () => {
    expect(validateImageFileForUpload({ type: 'text/html', name: 'poc.html', size: 1024 })).toBe(
      '图片格式不支持，请上传 JPG、PNG、WebP、HEIC、HEIF'
    )
  })

  it('allows camera files with missing MIME when the extension is supported', () => {
    expect(validateImageFileForUpload({ type: '', name: 'IMG_0001.HEIC', size: 1024 })).toBeNull()
  })
})
