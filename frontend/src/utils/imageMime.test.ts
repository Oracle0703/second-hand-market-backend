import { describe, expect, it } from 'vitest'
import { normalizeImageMIME } from './imageMime'

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
})
