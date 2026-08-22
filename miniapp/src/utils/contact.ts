import Taro from '@tarojs/taro'
import { STORE_PHONE_NUMBER } from '../constants/contact'

type CallbackResult = {
  errMsg?: string
  errNo?: number
  errno?: number
  errCode?: number
}

type PrivacySettingResult = CallbackResult & {
  needAuthorization?: boolean
  privacyContractName?: string
}

type PrivacyAPI = {
  getPrivacySetting?: (options: {
    success?: (result: PrivacySettingResult) => void
    fail?: (error: CallbackResult) => void
    complete?: (result: CallbackResult) => void
  }) => void
  requirePrivacyAuthorize?: (options: {
    success?: (result: CallbackResult) => void
    fail?: (error: CallbackResult) => void
    complete?: (result: CallbackResult) => void
  }) => void
  makePhoneCall?: typeof Taro.makePhoneCall
  showModal?: typeof Taro.showModal
  showToast?: typeof Taro.showToast
  setClipboardData?: typeof Taro.setClipboardData
}

type ContactRuntimeGlobal = {
  wx?: PrivacyAPI
  tt?: PrivacyAPI
}

type EnsurePrivacyOptions = {
  showFailure?: boolean
}

let phonePrivacyAuthorizedInSession = false

function runtimeGlobal(): ContactRuntimeGlobal {
  if (typeof globalThis === 'undefined') {
    return {}
  }
  return globalThis as ContactRuntimeGlobal
}

function privacyAPI(): PrivacyAPI {
  const taroAPI = Taro as unknown as PrivacyAPI
  const globalAPI = runtimeGlobal()
  return {
    ...globalAPI.wx,
    ...globalAPI.tt,
    ...taroAPI
  }
}

function resultCode(error: unknown): number | undefined {
  const value = error as CallbackResult | undefined
  return value?.errNo ?? value?.errno ?? value?.errCode
}

function resultMessage(error: unknown): string {
  if (error instanceof Error) {
    return error.message
  }
  const value = error as CallbackResult | undefined
  return value?.errMsg || String(error ?? '')
}

function privacyErrorMessage(error: unknown): string {
  const code = resultCode(error)
  const message = resultMessage(error)
  if (code === 113279) {
    return '当前小程序尚未完成拨打电话隐私声明，请联系商家配置后再试。'
  }
  if (code === 113280 || /privacy|隐私|authorize|auth/i.test(message)) {
    return '请先同意小程序隐私授权后再拨打电话。'
  }
  if (/cancel|取消/i.test(message)) {
    return '已取消拨号。'
  }
  return '暂时无法拉起拨号，请手动拨打商家电话。'
}

function phoneCallErrorMessage(error: unknown): string {
  const message = resultMessage(error)
  if (/cancel|取消/i.test(message)) {
    return '已取消拨号。'
  }
  return privacyErrorMessage(error)
}

async function showPhoneFallback(message: string): Promise<void> {
  const modal = await Taro.showModal({
    title: '无法拉起拨号',
    content: `${message}\n\n电话：${STORE_PHONE_NUMBER}`,
    confirmText: '复制号码',
    cancelText: '知道了'
  })
  if (!modal.confirm) {
    return
  }
  try {
    await Taro.setClipboardData({ data: STORE_PHONE_NUMBER })
  } catch {
    await Taro.showToast({ title: STORE_PHONE_NUMBER, icon: 'none' })
  }
}

function getPrivacySetting(): Promise<PrivacySettingResult> {
  const api = privacyAPI()
  if (typeof api.getPrivacySetting !== 'function') {
    return Promise.resolve({ needAuthorization: false })
  }
  return new Promise((resolve, reject) => {
    api.getPrivacySetting?.({
      success: resolve,
      fail: reject
    })
  })
}

function requirePrivacyAuthorize(): Promise<void> {
  const api = privacyAPI()
  if (typeof api.requirePrivacyAuthorize !== 'function') {
    return Promise.resolve()
  }
  return new Promise((resolve, reject) => {
    api.requirePrivacyAuthorize?.({
      success: () => resolve(),
      fail: reject
    })
  })
}

export async function ensureStorePhonePrivacyAuthorized(options: EnsurePrivacyOptions = {}): Promise<boolean> {
  if (phonePrivacyAuthorizedInSession) {
    return true
  }

  try {
    const setting = await getPrivacySetting()
    if (!setting.needAuthorization) {
      phonePrivacyAuthorizedInSession = true
      return true
    }
    await requirePrivacyAuthorize()
    phonePrivacyAuthorizedInSession = true
    return true
  } catch (error) {
    if (options.showFailure) {
      await showPhoneFallback(privacyErrorMessage(error))
    }
    return false
  }
}

export async function warmupStorePhonePrivacyAuthorization(): Promise<void> {
  const authorized = await ensureStorePhonePrivacyAuthorized()
  if (!authorized) {
    await Taro.showToast({ title: '未授权拨打电话，后续联系商家时可重新授权', icon: 'none' })
  }
}

export async function promptAndCallStore(): Promise<void> {
  const modal = await Taro.showModal({
    title: '联系商家',
    content: `是否拨打 ${STORE_PHONE_NUMBER} 这个电话？`,
    confirmText: '确定',
    cancelText: '取消'
  })

  if (!modal.confirm) {
    return
  }

  const authorized = await ensureStorePhonePrivacyAuthorized({ showFailure: true })
  if (!authorized) {
    return
  }

  try {
    await Taro.makePhoneCall({ phoneNumber: STORE_PHONE_NUMBER })
  } catch (error) {
    await showPhoneFallback(phoneCallErrorMessage(error))
  }
}
