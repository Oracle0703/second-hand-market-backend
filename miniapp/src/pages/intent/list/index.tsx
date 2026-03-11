import React, { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import Taro, { useDidShow } from '@tarojs/taro'
import { Picker, Text, View } from '@tarojs/components'
import { listIntents } from '../../../services/buyer'
import { requireLoginFor } from '../../../hooks/useRequireLogin'

const FILTERS = [
  { label: '全部', value: '' },
  { label: '处理中', value: 'NEW' },
  { label: '已联系', value: 'CONTACTED' },
  { label: '已关闭', value: 'CLOSED' }
]

export default function IntentListPage() {
  const [idx, setIdx] = useState(0)
  const status = FILTERS[idx].value

  useDidShow(() => {
    requireLoginFor('/pages/intent/list/index')
  })

  const query = useQuery({
    queryKey: ['buyer-intents', status],
    queryFn: () => listIntents({ status, page: 1, page_size: 50 })
  })

  return (
    <View className="page">
      <View className="card" style={{ marginBottom: '16rpx' }}>
        <Text style={{ marginRight: '10rpx' }}>筛选状态</Text>
        <Picker mode="selector" range={FILTERS.map((it) => it.label)} onChange={(e) => setIdx(Number(e.detail.value))}>
          <Text className="status-badge">{FILTERS[idx].label}</Text>
        </Picker>
      </View>

      <View className="card">
        {(query.data?.items || []).map((item) => (
          <View key={item.id} className="list-item" onClick={() => Taro.navigateTo({ url: `/pages/product/detail/index?id=${item.product.id}` })}>
            <Text>{item.product.title}</Text>
            <Text style={{ marginLeft: '10rpx' }} className="status-badge">
              {item.buyer_status_text}
            </Text>
          </View>
        ))}
        {(query.data?.items || []).length === 0 && !query.isLoading ? <View className="empty">暂无意向记录</View> : null}
      </View>
    </View>
  )
}
