import Taro from '@tarojs/taro'
import { STORE_PHONE_NUMBER } from '../constants/contact'

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

  await Taro.makePhoneCall({ phoneNumber: STORE_PHONE_NUMBER })
}
