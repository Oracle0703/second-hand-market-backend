import React, { useState } from 'react'
import Taro, { useRouter } from '@tarojs/taro'
import { Button, Text, View } from '@tarojs/components'
import { loginByWechat, mergeGuest } from '../../services/buyer'
import { ensureDeviceID, useSessionStore } from '../../stores/session'

export default function LoginPage() {
  const router = useRouter()
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const onLogin = async () => {
    setLoading(true)
    setError('')
    try {
      const loginRes = await Taro.login()
      const code = loginRes.code || `mock_${Date.now()}`
      const deviceID = ensureDeviceID()
      const data = await loginByWechat({ code, device_id: deviceID, nickname: '微信买家' })
      useSessionStore.getState().setSession(data.access_token, data.refresh_token, data.user)
      await mergeGuest(deviceID)

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
        <View className="title">微信登录</View>
        <Text style={{ color: '#6f7c77' }}>登录后可提交意向并同步游客收藏/浏览记录</Text>
        <Button className="btn-primary" style={{ marginTop: '20rpx' }} loading={loading} onClick={onLogin}>
          授权登录
        </Button>
        {error ? <View style={{ marginTop: '12rpx', color: '#b63732' }}>{error}</View> : null}
      </View>
    </View>
  )
}
