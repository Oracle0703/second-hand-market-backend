import Taro from '@tarojs/taro'
import { isLoggedIn } from '../stores/session'

export function requireLoginFor(targetPath: string): boolean {
  if (isLoggedIn()) {
    return true
  }
  const encoded = encodeURIComponent(targetPath)
  Taro.navigateTo({ url: `/pages/login/index?redirect=${encoded}` })
  return false
}
