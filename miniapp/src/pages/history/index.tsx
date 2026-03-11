import React from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import Taro from '@tarojs/taro'
import { Button, Text, View } from '@tarojs/components'
import { clearHistories, listHistories } from '../../services/buyer'

export default function HistoryPage() {
  const queryClient = useQueryClient()
  const query = useQuery({ queryKey: ['buyer-histories'], queryFn: () => listHistories(1, 50) })
  const clearMutation = useMutation({
    mutationFn: async () => clearHistories(),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['buyer-histories'] })
    }
  })

  return (
    <View className="page">
      <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <View className="title">浏览记录</View>
        <Button className="btn-secondary" size="mini" onClick={() => clearMutation.mutate()}>
          清空
        </Button>
      </View>

      {query.isLoading ? <Text>加载中...</Text> : null}
      {query.error ? <Text>{(query.error as Error).message}</Text> : null}

      <View className="card">
        {(query.data?.items || []).map((item: any) => (
          <View key={item.product_id} className="list-item" onClick={() => Taro.navigateTo({ url: `/pages/product/detail/index?id=${item.product_id}` })}>
            <Text>{item.title}</Text>
            <Text style={{ marginLeft: '12rpx', color: '#6f7c77' }}>浏览 {item.view_count} 次</Text>
          </View>
        ))}
        {(query.data?.items || []).length === 0 && !query.isLoading ? <View className="empty">暂无浏览记录</View> : null}
      </View>
    </View>
  )
}
