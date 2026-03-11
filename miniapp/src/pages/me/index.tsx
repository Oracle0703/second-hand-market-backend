import React from 'react'
import { useQuery } from '@tanstack/react-query'
import Taro, { useDidShow } from '@tarojs/taro'
import { Button, Text, View } from '@tarojs/components'
import { fetchSummary, logoutBuyer } from '../../services/buyer'
import { useSessionStore } from '../../stores/session'

export default function MePage() {
  const clearSession = useSessionStore((state) => state.clearSession)
  const summary = useQuery({ queryKey: ['buyer-summary'], queryFn: fetchSummary })

  useDidShow(() => {
    summary.refetch()
  })

  const logout = async () => {
    try {
      await logoutBuyer()
    } catch {
      // ignore logout network failures
    }
    clearSession()
    summary.refetch()
  }

  const data = summary.data
  return (
    <View className="page">
      <View className="card" style={{ marginBottom: '16rpx' }}>
        <View className="title">我的</View>
        {data?.is_login ? (
          <Text>{data.profile?.nickname || '微信买家'}</Text>
        ) : (
          <Text>当前为游客模式</Text>
        )}
      </View>

      <View className="card" style={{ marginBottom: '16rpx' }}>
        <View className="list-item">收藏: {data?.counters?.favorites ?? 0}</View>
        <View className="list-item" onClick={() => Taro.navigateTo({ url: '/pages/history/index' })}>
          浏览记录: {data?.counters?.histories ?? 0}
        </View>
        <View className="list-item" onClick={() => Taro.navigateTo({ url: '/pages/intent/list/index' })}>
          未关闭意向: {data?.counters?.intents_open ?? 0}
        </View>
      </View>

      {!data?.is_login ? (
        <Button className="btn-primary" onClick={() => Taro.navigateTo({ url: '/pages/login/index?redirect=%2Fpages%2Fme%2Findex' })}>
          去登录
        </Button>
      ) : (
        <Button className="btn-secondary" onClick={logout}>
          退出登录
        </Button>
      )}
    </View>
  )
}
