import React, { useState } from 'react'
import Taro, { useRouter } from '@tarojs/taro'
import { Button, Text, View } from '@tarojs/components'
import { loginByMiniProgram, mergeGuest } from '../../services/buyer'
import { ensureDeviceID, useSessionStore } from '../../stores/session'
import { getMiniProgramPlatform } from '../../utils/platform'

export default function LoginPage() {
  const router = useRouter()
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const platform = getMiniProgramPlatform()

  const onLogin = async () => {
    setLoading(true)
    setError('')
    try {
      const deviceID = ensureDeviceID()
      const startingVersion = useSessionStore.getState().sessionVersion
      const loginRes = await Taro.login()
      const code = loginRes.code || `mock_${Date.now()}`
      const data = await loginByMiniProgram({
        provider: platform.provider,
        code,
        device_id: deviceID,
        nickname: platform.defaultNickname
      })
      const beforeLogin = useSessionStore.getState()
      if (beforeLogin.sessionVersion !== startingVersion) return
      const loginVersion = beforeLogin.setSession(data.access_token, data.refresh_token, data.user)
      if (useSessionStore.getState().sessionVersion !== loginVersion) return
      let syncFailed = false
      try {
        await mergeGuest(deviceID)
      } catch {
        syncFailed = true
      }

      const current = useSessionStore.getState()
      if (!current.accessToken) {
        setError('登录已过期，请重新登录')
        return
      }
      if (current.sessionVersion !== loginVersion) return
      if (syncFailed) void Taro.showToast({ title: '游客数据同步失败', icon: 'none' })
      const redirect = router.params.redirect ? decodeURIComponent(router.params.redirect) : '/pages/me/index'
      if (redirect.startsWith('/pages/')) {
        Taro.redirectTo({ url: redirect })
      } else {
        Taro.switchTab({ url: '/pages/me/index' })
      }
    } catch (e) {
      setError((e as Error).message || '登录失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <View className="page">
      <View className="card" style={{ textAlign: 'center' }}>
        <View className="title">{platform.loginTitle}</View>
        <Text style={{ color: '#6f7c77' }}>登录后可同步游客收藏与浏览记录</Text>
        <Button className="btn-primary" style={{ marginTop: '20rpx' }} loading={loading} onClick={onLogin}>
          授权登录
        </Button>
        {error ? <View style={{ marginTop: '12rpx', color: '#b63732' }}>{error}</View> : null}
      </View>
    </View>
  )
}
