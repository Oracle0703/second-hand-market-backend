import { describe, expect, test } from 'vitest'
import { resolveAssetURL } from '../src/utils/url'

describe('小程序图片地址解析', () => {
  test('将后端 /uploads 路径绑定到 API 源站', () => {
    expect(
      resolveAssetURL('/uploads/product_image/example.jpg', 'https://market.example.com/api/v1')
    ).toBe('https://market.example.com/uploads/product_image/example.jpg')
  })

  test('支持不带前导斜杠的上传路径', () => {
    expect(
      resolveAssetURL('uploads/product_image/example.webp', 'http://127.0.0.1:8080/api/v1')
    ).toBe('http://127.0.0.1:8080/uploads/product_image/example.webp')
  })

  test.each([
    'https://cdn.example.com/example.jpg',
    'http://cdn.example.com/example.jpg',
    'data:image/png;base64,AAAA',
    'blob:https://app.example.com/id',
    'wxfile://temporary-image',
    'ttfile://temporary-image',
    '//cdn.example.com/example.jpg',
    '/assets/local-placeholder.png',
    './assets/local-placeholder.png',
    'assets/local-placeholder.png',
    '../assets/local-placeholder.png'
  ])('保留绝对地址和本地资源 %s', (url) => {
    expect(resolveAssetURL(url, 'https://market.example.com/api/v1')).toBe(url)
  })

  test('API 基址不是绝对地址时保持后端路径不变', () => {
    expect(resolveAssetURL('/uploads/example.jpg', '/api/v1')).toBe('/uploads/example.jpg')
  })

  test.each([
    'javascript:alert(1)',
    'data:text/html,<script>alert(1)</script>',
    'file:///etc/passwd'
  ])('拒绝非图片或未知协议 %s', (url) => {
    expect(resolveAssetURL(url, 'https://market.example.com/api/v1')).toBe('')
  })

  test('API 基址带端口、路径和查询参数时只使用源站', () => {
    expect(
      resolveAssetURL('/uploads/example.jpg', 'https://market.example.com:8443/api/v1?tenant=demo')
    ).toBe('https://market.example.com:8443/uploads/example.jpg')
  })
})
