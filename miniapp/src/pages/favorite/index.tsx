import React from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import Taro from '@tarojs/taro'
import { Button, Text, View } from '@tarojs/components'
import { BuyerFavoriteItem, listFavorites, removeFavorite } from '../../services/buyer'
import { centToYuanText } from '../../utils/price'

export default function FavoritePage() {
  const queryClient = useQueryClient()
  const query = useQuery({
    queryKey: ['buyer-favorites'],
    queryFn: () => listFavorites(1, 50)
  })

  const removeMutation = useMutation({
    mutationFn: async (productID: number) => removeFavorite(productID),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['buyer-favorites'] })
    }
  })

  return (
    <View className="page">
      <View className="title">我的收藏</View>
      {query.isLoading ? <Text>加载中...</Text> : null}
      {query.error ? <Text>{(query.error as Error).message}</Text> : null}
      <View className="card">
        {(query.data?.items || []).map((item: BuyerFavoriteItem) => (
          <View key={item.product_id} className="list-item">
            <View onClick={() => Taro.navigateTo({ url: `/pages/product/detail/index?id=${item.product_id}` })}>
              <Text>{item.title}</Text>
              <View style={{ marginTop: '8rpx', display: 'flex', alignItems: 'center', flexWrap: 'wrap' }}>
                <Text style={{ color: '#d24b2f' }}>售价 ¥{centToYuanText(item.price_cent)}</Text>
                <Text style={{ marginLeft: '12rpx', color: '#8a9691', textDecoration: 'line-through' }}>
                  原价 ¥{centToYuanText(item.original_price_cent ?? item.price_cent)}
                </Text>
                <Text style={{ marginLeft: '12rpx', color: '#6f7c77' }}>仅剩 {item.stock} 件</Text>
              </View>
            </View>
            <Button className="btn-secondary" size="mini" style={{ marginTop: '10rpx' }} onClick={() => removeMutation.mutate(item.product_id)}>
              取消收藏
            </Button>
          </View>
        ))}
        {(query.data?.items || []).length === 0 && !query.isLoading ? <View className="empty">暂无收藏</View> : null}
      </View>
    </View>
  )
}
