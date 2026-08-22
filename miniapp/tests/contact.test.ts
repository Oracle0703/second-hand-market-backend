import { beforeEach, describe, expect, test, vi } from 'vitest'

const showModal = vi.fn(async () => ({ confirm: true }))
const makePhoneCall = vi.fn(async () => ({}))

vi.mock('@tarojs/taro', () => ({
  default: {
    showModal,
    makePhoneCall
  }
}))

describe('联系商家电话', () => {
  beforeEach(() => {
    showModal.mockClear()
    makePhoneCall.mockClear()
    showModal.mockResolvedValue({ confirm: true })
  })

  test('确认联系时展示并拨打最新商家电话', async () => {
    const { promptAndCallStore } = await import('../src/utils/contact')

    await promptAndCallStore()

    expect(showModal).toHaveBeenCalledWith(expect.objectContaining({
      content: '是否拨打 15008387726 这个电话？'
    }))
    expect(makePhoneCall).toHaveBeenCalledWith({ phoneNumber: '15008387726' })
  })

  test('取消联系时不拨打电话', async () => {
    showModal.mockResolvedValue({ confirm: false })
    const { promptAndCallStore } = await import('../src/utils/contact')

    await promptAndCallStore()

    expect(makePhoneCall).not.toHaveBeenCalled()
  })
})
