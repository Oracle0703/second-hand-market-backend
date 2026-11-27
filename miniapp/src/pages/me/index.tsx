import React, { useEffect } from 'react'
import { useQuery, useQueryClient } from '@/libs/react-query'
import Taro, { useDidShow } from '@tarojs/taro'
import { usePageExposure } from '@/hooks/usePageExposure'
import { Button, Text, View } from '@tarojs/components'
import { fetchSummary, logoutBuyer } from '../../services/buyer'
import { ensureDeviceID, useSessionStore } from '../../stores/session'
import { getMiniProgramPlatform } from '../../utils/platform'

export default function MePage() {
  usePageExposure('pages/me/index')
  const owner = useSessionStore((state) => {
    if (!state.hydrated) return null
    if (state.accessToken) return `buyer:${state.profile?.id ?? 'unknown'}:${state.sessionVersion}`
    return state.deviceID ? `guest:${state.deviceID}` : null
  })
  const client = useQueryClient()
  useEffect(() => {
    ensureDeviceID()
  }, [])

  useDidShow(() => {
    if (owner) void client.invalidateQueries({ queryKey: ['buyer-summary', owner] })
  })

  return owner ? <OwnerSummary key={owner} owner={owner} /> : <View className="page">加载账户信息…</View>
}

function OwnerSummary({ owner }: { owner: string }) {
  const summary = useQuery({ queryKey: ['buyer-summary', owner], queryFn: fetchSummary })
  const platform = getMiniProgramPlatform()
  const reloadSummary = () => { void summary.refetch().catch(() => undefined) }
  const logout = async () => {
    const version = useSessionStore.getState().sessionVersion
    try {
      await logoutBuyer()
    } catch {
      // ignore logout network failures
    }
    const current = useSessionStore.getState()
    if (current.sessionVersion === version) current.clearSession()
  }

  const data = summary.data
  if (!data && !summary.error) return <View className="page">加载账户信息…</View>
  const loadFailed = Boolean(summary.error) && !data
  return (
    <View className="page">
      <View className="card" style={{ marginBottom: '16rpx' }}>
        <View className="title">我的</View>
        {loadFailed ? (
          <Text>账户信息加载失败</Text>
        ) : data?.is_login ? (
          <Text>{data.profile?.nickname || platform.defaultNickname}</Text>
        ) : (
          <Text>当前为游客模式</Text>
        )}
        {summary.error ? <Text>{(summary.error as Error).message || '加载失败，请重试'}</Text> : null}
      </View>

      <View className="card" style={{ marginBottom: '16rpx' }}>
        <View className="list-item">收藏: {loadFailed ? '--' : data?.counters?.favorites ?? 0}</View>
        <View className="list-item" onClick={() => Taro.navigateTo({ url: '/pages/history/index' })}>
          浏览记录: {loadFailed ? '--' : data?.counters?.histories ?? 0}
        </View>
      </View>

      {loadFailed ? (
        <Button className="btn-primary" onClick={reloadSummary}>
          重试
        </Button>
      ) : !data?.is_login ? (
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
