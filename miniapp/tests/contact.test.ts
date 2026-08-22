import { beforeEach, describe, expect, test, vi } from 'vitest'

const showModal = vi.fn(async () => ({ confirm: true }))
const makePhoneCall = vi.fn(async () => ({}))
const getPrivacySetting = vi.fn(({ success }) => success({ needAuthorization: false }))
const requirePrivacyAuthorize = vi.fn(({ success }) => success({ errMsg: 'ok' }))
const setClipboardData = vi.fn(async () => ({}))
const showToast = vi.fn(async () => ({}))

vi.mock('@tarojs/taro', () => ({
  default: {
    showModal,
    makePhoneCall,
    getPrivacySetting,
    requirePrivacyAuthorize,
    setClipboardData,
    showToast
  }
}))

describe('联系商家电话', () => {
  beforeEach(() => {
    vi.resetModules()
    showModal.mockClear()
    makePhoneCall.mockClear()
    getPrivacySetting.mockClear()
    requirePrivacyAuthorize.mockClear()
    setClipboardData.mockClear()
    showToast.mockClear()
    showModal.mockResolvedValue({ confirm: true })
    getPrivacySetting.mockImplementation(({ success }) => success({ needAuthorization: false }))
    requirePrivacyAuthorize.mockImplementation(({ success }) => success({ errMsg: 'ok' }))
    makePhoneCall.mockResolvedValue({})
    setClipboardData.mockResolvedValue({})
    showToast.mockResolvedValue({})
  })

  test('确认联系时展示并拨打最新商家电话', async () => {
    const { promptAndCallStore } = await import('../src/utils/contact')

    await promptAndCallStore()

    expect(showModal).toHaveBeenCalledWith(expect.objectContaining({
      content: '是否拨打 15008387726 这个电话？'
    }))
    expect(getPrivacySetting).toHaveBeenCalled()
    expect(requirePrivacyAuthorize).not.toHaveBeenCalled()
    expect(makePhoneCall).toHaveBeenCalledWith({ phoneNumber: '15008387726' })
  })

  test('拨号前需要隐私授权时先触发授权', async () => {
    getPrivacySetting.mockImplementation(({ success }) => success({ needAuthorization: true }))
    const { promptAndCallStore } = await import('../src/utils/contact')

    await promptAndCallStore()

    expect(requirePrivacyAuthorize).toHaveBeenCalled()
    expect(makePhoneCall).toHaveBeenCalledWith({ phoneNumber: '15008387726' })
  })

  test('同一次小程序会话授权成功后不重复触发隐私授权', async () => {
    getPrivacySetting.mockImplementation(({ success }) => success({ needAuthorization: true }))
    const { promptAndCallStore } = await import('../src/utils/contact')

    await promptAndCallStore()
    await promptAndCallStore()

    expect(getPrivacySetting).toHaveBeenCalledTimes(1)
    expect(requirePrivacyAuthorize).toHaveBeenCalledTimes(1)
    expect(makePhoneCall).toHaveBeenCalledTimes(2)
  })

  test('取消联系时不拨打电话', async () => {
    showModal.mockResolvedValue({ confirm: false })
    const { promptAndCallStore } = await import('../src/utils/contact')

    await promptAndCallStore()

    expect(makePhoneCall).not.toHaveBeenCalled()
  })

  test('隐私授权失败时提示并不拨号', async () => {
    getPrivacySetting.mockImplementation(({ success }) => success({ needAuthorization: true }))
    requirePrivacyAuthorize.mockImplementation(({ fail }) => fail({ errNo: 113280, errMsg: 'privacy permission is not authorized' }))
    const { promptAndCallStore } = await import('../src/utils/contact')

    await promptAndCallStore()

    expect(makePhoneCall).not.toHaveBeenCalled()
    expect(showModal).toHaveBeenLastCalledWith(expect.objectContaining({
      title: '无法拉起拨号',
      content: expect.stringContaining('请先同意小程序隐私授权后再拨打电话')
    }))
  })

  test('平台拨号失败时提示并允许复制号码', async () => {
    makePhoneCall.mockRejectedValue({ errNo: 113279, errMsg: 'privacy declaration missing' })
    const { promptAndCallStore } = await import('../src/utils/contact')

    await promptAndCallStore()

    expect(showModal).toHaveBeenLastCalledWith(expect.objectContaining({
      title: '无法拉起拨号',
      confirmText: '复制号码'
    }))
    expect(setClipboardData).toHaveBeenCalledWith({ data: '15008387726' })
  })

  test('登录后可预触发隐私授权且失败不抛出', async () => {
    getPrivacySetting.mockImplementation(({ success }) => success({ needAuthorization: true }))
    requirePrivacyAuthorize.mockImplementation(({ fail }) => fail({ errNo: 113280, errMsg: 'privacy permission is not authorized' }))
    const { warmupStorePhonePrivacyAuthorization } = await import('../src/utils/contact')

    await expect(warmupStorePhonePrivacyAuthorization()).resolves.toBeUndefined()

    expect(showToast).toHaveBeenCalledWith(expect.objectContaining({
      title: '未授权拨打电话，后续联系商家时可重新授权'
    }))
  })
})
