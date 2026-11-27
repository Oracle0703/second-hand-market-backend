import React from 'react'
import { useQuery, useQueryClient } from '@/libs/react-query'
import { useToastMutation } from '@/hooks/useToastMutation'
import Taro, { useDidShow } from '@tarojs/taro'
import { Button, Text, View } from '@tarojs/components'
import { usePageExposure } from '@/hooks/usePageExposure'
import { trackClick } from '@/services/analytics'
import { BuyerFavoriteItem, listFavorites, removeFavorite } from '../../services/buyer'
import { promptAndCallStore } from '../../utils/contact'
import { centToYuanText } from '../../utils/price'
import { canContactForProduct, getProductStatusText } from '../../utils/product-status'

const PAGE = 'pages/favorite/index'

export default function FavoritePage() {
  usePageExposure(PAGE)
  const queryClient = useQueryClient()
  const query = useQuery({
    queryKey: ['buyer-favorites'],
    queryFn: () => listFavorites(1, 50)
  })
  const hasShownRef = React.useRef(false)

  useDidShow(() => {
    if (!hasShownRef.current) {
      hasShownRef.current = true
      return
    }
    void query.refetch().catch(() => undefined)
  })

  const removeMutation = useToastMutation({
    mutationFn: async (productID: number) => removeFavorite(productID),
    errorMessage: '取消收藏失败，请重试',
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
            <View onClick={() => {
              trackClick('product_click', PAGE, 'favorite_list', item.product_id)
              Taro.navigateTo({ url: `/pages/product/detail/index?id=${item.product_id}` })
            }}>
              <Text>{item.title}</Text>
              <View style={{ marginTop: '8rpx', display: 'flex', alignItems: 'center', flexWrap: 'wrap' }}>
                <Text style={{ color: '#d24b2f' }}>售价 ¥{centToYuanText(item.price_cent)}</Text>
                <Text style={{ marginLeft: '12rpx', color: '#8a9691', textDecoration: 'line-through' }}>
                  原价 ¥{centToYuanText(item.original_price_cent ?? item.price_cent)}
                </Text>
                <Text style={{ marginLeft: '12rpx', color: '#6f7c77' }}>仅剩 {item.stock} 件</Text>
                <Text className="status-badge" style={{ marginLeft: '12rpx' }}>{getProductStatusText(item.status)}</Text>
              </View>
            </View>
            <View style={{ marginTop: '10rpx', display: 'flex', gap: '12rpx' }}>
              <Button
                className="btn-secondary"
                size="mini"
                disabled={removeMutation.isPending}
                loading={removeMutation.isPending}
                onClick={() => {
                  trackClick('favorite_click', PAGE, 'favorite_unfavorite', item.product_id)
                  removeMutation.mutate(item.product_id)
                }}
              >
                取消收藏
              </Button>
              {canContactForProduct(item.status) ? (
                <Button className="btn-primary" size="mini" onClick={() => void promptAndCallStore(PAGE, 'favorite_want', item.product_id)}>
                  我想要
                </Button>
              ) : null}
            </View>
          </View>
        ))}
        {(query.data?.items || []).length === 0 && !query.isLoading ? <View className="empty">暂无收藏</View> : null}
      </View>
    </View>
  )
}
