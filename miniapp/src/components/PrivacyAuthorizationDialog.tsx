import React, { useEffect, useState } from 'react'
import Taro from '@tarojs/taro'
import { Button, Text, View } from '@tarojs/components'
import { openPrivacyContract, registerPrivacyAuthorizationListener } from '../utils/contact'

const AGREE_BUTTON_ID = 'privacy-authorization-agree'

type PrivacyResolution = {
  event: 'exposureAuthorization' | 'agree' | 'disagree'
  buttonId?: string
}

type PendingAuthorization = {
  resolve: (result: PrivacyResolution) => void
}

export default function PrivacyAuthorizationDialog() {
  const [pendingAuthorization, setPendingAuthorization] = useState<PendingAuthorization | null>(null)

  useEffect(() => registerPrivacyAuthorizationListener((resolve) => {
    setPendingAuthorization({ resolve })
  }), [])

  useEffect(() => {
    pendingAuthorization?.resolve({ event: 'exposureAuthorization' })
  }, [pendingAuthorization])

  if (!pendingAuthorization) {
    return null
  }

  const finishAuthorization = (result: PrivacyResolution) => {
    const current = pendingAuthorization
    setPendingAuthorization(null)
    current.resolve(result)
  }

  const handleOpenPrivacyContract = async () => {
    try {
      await openPrivacyContract()
    } catch {
      await Taro.showToast({ title: '暂时无法打开隐私保护指引', icon: 'none' })
    }
  }

  return (
    <View className="privacy-auth-backdrop">
      <View className="privacy-auth-dialog">
        <Text className="privacy-auth-title">隐私保护提示</Text>
        <Text className="privacy-auth-content">
          为向你提供拨打商家电话功能，小程序需要在你主动使用时调用设备拨号能力。请阅读并同意小程序隐私保护指引。
        </Text>
        <Button className="privacy-auth-contract" onClick={() => void handleOpenPrivacyContract()}>
          查看隐私保护指引
        </Button>
        <View className="privacy-auth-actions">
          <Button
            className="privacy-auth-button privacy-auth-reject"
            onClick={() => finishAuthorization({ event: 'disagree' })}
          >
            暂不同意
          </Button>
          <Button
            id={AGREE_BUTTON_ID}
            className="privacy-auth-button privacy-auth-agree"
            onClick={() => finishAuthorization({ event: 'agree', buttonId: AGREE_BUTTON_ID })}
          >
            同意并继续
          </Button>
        </View>
      </View>
    </View>
  )
}
