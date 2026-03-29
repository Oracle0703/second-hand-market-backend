import React, { useMemo, useRef, useState } from 'react'
import Taro, { useDidShow, useReachBottom } from '@tarojs/taro'
import { Button, Image, Swiper, SwiperItem, Text, View } from '@tarojs/components'
import { BuyerProduct, fetchBuyerProducts } from '../../services/buyer'
import { centToYuanText } from '../../utils/price'

const PAGE_SIZE = 10
const PHONE_NUMBER = '13699479406'

function mergeProducts(current: BuyerProduct[], incoming: BuyerProduct[]): BuyerProduct[] {
  const seen = new Set<number>()
  const merged = [...current, ...incoming].filter((item) => {
    if (seen.has(item.id)) {
      return false
    }
    seen.add(item.id)
    return true
  })

  return merged
}

export default function HomePage() {
  const [items, setItems] = useState<BuyerProduct[]>([])
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [error, setError] = useState('')
  const initializedRef = useRef(false)
  const loadingRef = useRef(false)

  const hasMore = items.length < total
  const heroItems = useMemo(() => items.slice(0, 5), [items])

  const loadProducts = async (targetPage: number, replace = false) => {
    if (loadingRef.current) {
      return
    }

    loadingRef.current = true
    setError('')

    if (replace) {
      setLoading(true)
    } else {
      setLoadingMore(true)
    }

    try {
      const result = await fetchBuyerProducts({ page: targetPage, page_size: PAGE_SIZE, sort: 'latest' })
      const nextItems = result.items || []

      setTotal(result.total || 0)
      setPage(result.page || targetPage)
      setItems((prev) => (replace ? nextItems : mergeProducts(prev, nextItems)))
    } catch (err) {
      setError(err instanceof Error ? err.message : '商品加载失败')
    } finally {
      loadingRef.current = false
      setLoading(false)
      setLoadingMore(false)
    }
  }

  const reloadProducts = async () => {
    await loadProducts(1, true)
  }

  const goDetail = (id: number) => {
    Taro.navigateTo({ url: `/pages/product/detail/index?id=${id}` })
  }

  const handleWant = async () => {
    const modal = await Taro.showModal({
      title: '联系商家',
      content: `是否拨打 ${PHONE_NUMBER} 这个电话？`,
      confirmText: '确定',
      cancelText: '取消'
    })

    if (!modal.confirm) {
      return
    }

    await Taro.makePhoneCall({ phoneNumber: PHONE_NUMBER })
  }

  useDidShow(() => {
    if (initializedRef.current) {
      void reloadProducts()
      return
    }

    initializedRef.current = true
    void reloadProducts()
  })

  useReachBottom(() => {
    if (!loadingRef.current && hasMore) {
      void loadProducts(page + 1)
    }
  })

  return (
    <View className="page home-page">
      <View className="home-hero">
        <View className="home-hero-copy">
          <Text className="home-hero-kicker">瑞扬二手家具</Text>
          <Text className="home-hero-title">在售好物持续上新</Text>
          <Text className="home-hero-desc">精选在售商品，看到喜欢的直接拨号联系，省去来回沟通。</Text>
        </View>

        {heroItems.length > 0 ? (
          <Swiper className="home-swiper" indicatorDots autoplay circular interval={3500} duration={500}>
            {heroItems.map((item) => (
              <SwiperItem key={item.id}>
                <View className="home-swiper-slide" onClick={() => goDetail(item.id)}>
                  {item.cover_url ? (
                    <Image className="home-swiper-image" src={item.cover_url} mode="aspectFill" />
                  ) : (
                    <View className="home-swiper-fallback">
                      <Text className="home-swiper-fallback-kicker">在售商品</Text>
                      <Text className="home-swiper-fallback-name">{item.title}</Text>
                    </View>
                  )}
                  <View className="home-swiper-mask">
                    <Text className="home-swiper-badge">上新在售</Text>
                    <Text className="home-swiper-name">{item.title}</Text>
                    <Text className="home-swiper-price">
                      到手价 ¥{centToYuanText(item.price_cent)}
                      {item.stock > 0 ? ` · 仅剩 ${item.stock} 件` : ''}
                    </Text>
                  </View>
                </View>
              </SwiperItem>
            ))}
          </Swiper>
        ) : (
          <View className="home-swiper home-swiper-empty">
            <Text>暂无轮播商品</Text>
          </View>
        )}
      </View>

      <View className="home-section-head">
        <Text className="title" style={{ marginBottom: 0 }}>在售商品</Text>
        <Text className="home-section-meta">{total > 0 ? `${total} 件商品` : '持续更新中'}</Text>
      </View>

      {loading ? <View className="empty">加载中...</View> : null}
      {error ? <View className="empty">{error}</View> : null}
      {!loading && !error && items.length === 0 ? <View className="empty">暂无在售商品</View> : null}

      <View className="home-grid">
        {items.map((item) => {
          const originalPriceCent = item.original_price_cent ?? item.price_cent

          return (
            <View key={item.id} className="home-product-card">
              <View className="home-product-cover-wrap" onClick={() => goDetail(item.id)}>
                {item.cover_url ? (
                  <Image className="home-product-cover" src={item.cover_url} mode="aspectFill" />
                ) : (
                  <View className="home-product-placeholder">
                    <Text className="home-product-placeholder-text">暂无图片</Text>
                  </View>
                )}
              </View>

              <View className="home-product-body">
                <Text className="home-product-title">{item.title}</Text>
                <View className="home-product-price-row">
                  <Text className="home-product-price">售价 ¥{centToYuanText(item.price_cent)}</Text>
                  <Text className="home-product-original">原价 ¥{centToYuanText(originalPriceCent)}</Text>
                </View>
                <View className="home-product-footer">
                  <Text className="home-product-stock">仅剩 {item.stock} 件</Text>
                  <Button
                    className="home-want-btn"
                    size="mini"
                    onClick={(event) => {
                      event.stopPropagation()
                      void handleWant()
                    }}
                  >
                    我想要
                  </Button>
                </View>
              </View>
            </View>
          )
        })}
      </View>

      {!loading && !error && items.length > 0 ? (
        <View className="home-loadmore">
          <Text>{loadingMore ? '正在加载更多商品...' : hasMore ? '下拉到底部继续加载' : '已经到底了'}</Text>
        </View>
      ) : null}
    </View>
  )
}
