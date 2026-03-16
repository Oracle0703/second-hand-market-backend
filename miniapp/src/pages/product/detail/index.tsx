import React from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import Taro, { useRouter, useShareAppMessage } from '@tarojs/taro'
import { Button, Image, Swiper, SwiperItem, Text, View } from '@tarojs/components'
import { addFavorite, fetchBuyerProductDetail, removeFavorite, reportView } from '../../../services/buyer'
import { requireLoginFor } from '../../../hooks/useRequireLogin'
import { centToYuanText } from '../../../utils/price'

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
  const imageURLs = (product?.images || []).filter(Boolean)

  useShareAppMessage(() => ({
    title: product?.title || '二手好物',
    path: `/pages/product/detail/index?id=${id}`
  }))

  const canSubmit = !!product?.can_submit_intent

  const handleIntent = () => {
    const target = `/pages/intent/create/index?product_id=${id}`
    if (!requireLoginFor(target)) {
      return
    }
    Taro.navigateTo({ url: target })
  }

  return (
    <View className="page">
      {detail.isLoading ? <Text>加载中...</Text> : null}
      {detail.error ? <Text>{(detail.error as Error).message}</Text> : null}
      {product ? (
        <View className="card">
          <View className="title">{product.title}</View>
          <View style={{ marginBottom: '12rpx' }}>
            <Text style={{ color: '#d24b2f', fontWeight: 600 }}>¥{centToYuanText(product.price_cent)}</Text>
            <Text style={{ marginLeft: '16rpx' }} className="status-badge">
              {product.status}
            </Text>
          </View>

          {imageURLs.length > 0 ? (
            <Swiper
              indicatorDots
              circular
              style={{ width: '100%', height: '420rpx', marginBottom: '12rpx' }}
            >
              {imageURLs.map((url, idx) => (
                <SwiperItem key={`${url}-${idx}`}>
                  <Image
                    src={url}
                    mode="aspectFill"
                    style={{ width: '100%', height: '420rpx', borderRadius: '12rpx' }}
                    onClick={() => Taro.previewImage({ current: url, urls: imageURLs })}
                  />
                </SwiperItem>
              ))}
            </Swiper>
          ) : product.cover_url ? (
            <Image
              src={product.cover_url}
              mode="aspectFill"
              style={{ width: '100%', height: '420rpx', borderRadius: '12rpx', marginBottom: '12rpx' }}
            />
          ) : null}

          <Text>{product.description}</Text>

          {!canSubmit ? <View style={{ marginTop: '16rpx', color: '#7d8b86' }}>当前商品不可提交意向，仅支持查看。</View> : null}

          <View className="toolbar" style={{ marginTop: '18rpx', display: 'flex', gap: '12rpx' }}>
            <Button
              className="btn-secondary"
              onClick={() => favoriteMutation.mutate(!product.is_favorited)}
              loading={favoriteMutation.isPending}
            >
              {product.is_favorited ? '取消收藏' : '收藏'}
            </Button>
            <Button className={canSubmit ? 'btn-primary' : 'btn-disabled'} disabled={!canSubmit} onClick={handleIntent}>
              提交意向
            </Button>
          </View>
        </View>
      ) : null}
    </View>
  )
}
