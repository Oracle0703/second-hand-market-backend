import { describe, expect, it } from 'vitest'
import { createDevProxy } from './config/devProxy'

describe('development asset proxy', () => {
  it('routes API and guarded uploads through the same backend', () => {
    const target = 'http://127.0.0.1:9080'
    const proxy = createDevProxy(target)

    expect(proxy['/api']).toEqual({ target, changeOrigin: true })
    expect(proxy['/uploads']).toEqual({ target, changeOrigin: true })
  })
})
