import React from 'react'
import { useMutation, useQuery, useQueryClient } from '@/libs/react-query'
import Taro, { useRouter, useShareAppMessage } from '@tarojs/taro'
import { Button, Image, Swiper, SwiperItem, Text, View } from '@tarojs/components'
import { addFavorite, fetchBuyerProductDetail, removeFavorite, reportView } from '../../../services/buyer'
import { promptAndCallStore } from '../../../utils/contact'
import { centToYuanText } from '../../../utils/price'
import { resolveAssetURL } from '../../../utils/url'

export default function ProductDetailPage() {
  const router = useRouter()
  const queryClient = useQueryClient()
  const id = Number(router.params.id || 0)

  const detail = useQuery({
    queryKey: ['buyer-product-detail', id],
    queryFn: async () => fetchBuyerProductDetail(id),
    enabled: id > 0
  })

  const favoriteMutation = useMutation({
    mutationFn: async (nextFav: boolean) => {
      if (nextFav) return addFavorite(id)
      return removeFavorite(id)
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['buyer-product-detail', id] })
      await queryClient.invalidateQueries({ queryKey: ['buyer-favorites'] })
    }
  })

  useQuery({
    queryKey: ['buyer-product-view-report', id],
    queryFn: async () => reportView(id),
    enabled: id > 0,
    retry: false
  })

  const product = detail.data?.product
  const imageURLs = (product?.images || []).map((url) => resolveAssetURL(url)).filter(Boolean)
  const originalPriceCent = product?.original_price_cent ?? product?.price_cent ?? 0
  const [activeImageIndex, setActiveImageIndex] = React.useState(0)

  useShareAppMessage(() => ({
    title: product?.title || '二手好物',
    path: `/pages/product/detail/index?id=${id}`
  }))

  return (
    <View className="page">
      {detail.isLoading ? <Text>加载中...</Text> : null}
      {detail.error ? <Text>{(detail.error as Error).message}</Text> : null}
      {product ? (
        <View className="card">
          <View className="title">{product.title}</View>
          <View style={{ marginBottom: '12rpx' }}>
            <View style={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap' }}>
              <Text style={{ color: '#d24b2f', fontWeight: 600 }}>售价 ¥{centToYuanText(product.price_cent)}</Text>
              <Text style={{ marginLeft: '12rpx', color: '#8a9691', textDecoration: 'line-through' }}>原价 ¥{centToYuanText(originalPriceCent)}</Text>
            </View>
            <View style={{ marginTop: '8rpx', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <Text style={{ color: '#6f7c77' }}>仅剩 {product.stock} 件</Text>
              <Text className="status-badge">{product.status}</Text>
            </View>
          </View>

          {imageURLs.length > 0 ? (
            <Swiper
              indicatorDots
              circular
              current={activeImageIndex}
              onChange={(event) => setActiveImageIndex(event.detail.current)}
              style={{ width: '100%', height: '420rpx', marginBottom: '12rpx' }}
            >
              {imageURLs.map((url, idx) => {
                const shouldLoadImage = Math.abs(idx - activeImageIndex) <= 1

                return (
                  <SwiperItem key={`${url}-${idx}`}>
                    <Image
                      src={shouldLoadImage ? url : ''}
                      mode="aspectFill"
                      style={{ width: '100%', height: '420rpx', borderRadius: '12rpx' }}
                      onClick={() => Taro.previewImage({ current: url, urls: imageURLs })}
                    />
                  </SwiperItem>
                )
              })}
            </Swiper>
          ) : product.cover_url ? (
            <Image
              src={resolveAssetURL(product.cover_url)}
              mode="aspectFill"
              style={{ width: '100%', height: '420rpx', borderRadius: '12rpx', marginBottom: '12rpx' }}
            />
          ) : null}

          <Text>{product.description}</Text>
          <View className="toolbar" style={{ marginTop: '18rpx', display: 'flex', gap: '12rpx' }}>
            <Button
              className="btn-secondary"
              onClick={() => favoriteMutation.mutate(!product.is_favorited)}
              loading={favoriteMutation.isPending}
            >
              {product.is_favorited ? '取消收藏' : '收藏'}
            </Button>
            <Button className="btn-primary" onClick={() => void promptAndCallStore()}>
              我想要
            </Button>
          </View>
        </View>
      ) : null}
    </View>
  )
}
