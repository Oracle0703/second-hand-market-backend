import { afterEach, describe, expect, test, vi } from 'vitest'

describe('session 启动安全性', () => {
  afterEach(() => {
    vi.resetModules()
    vi.unmock('@tarojs/taro')
  })

  test('导入 session 模块时不应立即访问存储，设备 ID 应按需初始化', async () => {
    const taroMock = {
      getStorageSync: vi.fn(() => ''),
      setStorageSync: vi.fn(),
      removeStorageSync: vi.fn()
    }

    vi.doMock('@tarojs/taro', () => ({
      default: taroMock
    }))

    const session = await import('../src/stores/session')

    expect(taroMock.getStorageSync).not.toHaveBeenCalled()
    expect(taroMock.setStorageSync).not.toHaveBeenCalled()

    const deviceID = session.ensureDeviceID()

    expect(deviceID).toMatch(/^dev_/)
    expect(taroMock.getStorageSync).toHaveBeenCalledWith('buyer_device_id')
    expect(taroMock.setStorageSync).toHaveBeenCalledWith('buyer_device_id', deviceID)
  })
})
