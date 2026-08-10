import React, { useMemo, useRef, useState } from 'react'
import Taro, { useDidShow, useReachBottom } from '@tarojs/taro'
import { Button, Image, Swiper, SwiperItem, Text, View } from '@tarojs/components'
import { BuyerProduct, fetchBuyerProducts } from '../../services/buyer'
import { hasStoreGuideVideo, hasStoreLocation, STORE_GUIDE_VIDEO, STORE_LOCATION } from '../../constants/store'
import { promptAndCallStore } from '../../utils/contact'
import { centToYuanText } from '../../utils/price'
import { resolveAssetURL } from '../../utils/url'

const PAGE_SIZE = 10

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
  const locationConfigured = hasStoreLocation(STORE_LOCATION)
  const storeGuideVideoConfigured = hasStoreGuideVideo(STORE_GUIDE_VIDEO)

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

  const showLocationFallback = async (title: string, content: string) => {
    await Taro.showModal({
      title,
      content: `${content}\n\n门店：${STORE_LOCATION.name}\n地址：${STORE_LOCATION.address}`,
      showCancel: false,
      confirmText: '知道了'
    })
  }

  const handleOpenLocation = async () => {
    if (!locationConfigured || STORE_LOCATION.latitude === null || STORE_LOCATION.longitude === null) {
      await Taro.showToast({ title: '请先配置门店地址', icon: 'none' })
      return
    }

    const platform = Taro.getSystemInfoSync().platform
    const isDevtools = platform === 'devtools'

    if (isDevtools) {
      await showLocationFallback('开发者工具限制', '微信开发者工具通常不能直接拉起地图，请在手机真机里点击“导航去店”测试。')
      return
    }

    try {
      await Taro.openLocation({
        latitude: STORE_LOCATION.latitude,
        longitude: STORE_LOCATION.longitude,
        name: STORE_LOCATION.name,
        address: STORE_LOCATION.address,
        scale: 18
      })
    } catch (error) {
      const errMsg = error instanceof Error ? error.message : String(error ?? '')
      await showLocationFallback(
        '地图打开失败',
        isDevtools
          ? '开发者工具通常不支持直接拉起地图，请在手机真机里重试。'
          : `暂时无法打开地图${errMsg ? `：${errMsg}` : ''}。请确认平台隐私授权后再试。`
      )
    }
  }

  const handleOpenStoreGuide = () => {
    Taro.navigateTo({ url: '/pages/store-guide/index' })
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
                    <Image className="home-swiper-image" src={resolveAssetURL(item.cover_url)} mode="aspectFill" />
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

      <View className={`home-location-card${locationConfigured ? '' : ' home-location-card-pending'}`}>
        <View className="home-location-copy">
          <Text className="home-location-label">到店地址</Text>
          <Text className="home-location-name">{STORE_LOCATION.name}</Text>
          <Text className="home-location-address">
            {locationConfigured ? STORE_LOCATION.address : '暂未配置门店地址，补充后可一键唤起导航。'}
          </Text>
        </View>

        <View className="home-location-actions">
          <Button
            className="home-location-btn home-location-btn-secondary"
            size="mini"
            onClick={(event) => {
              event.stopPropagation()
              void promptAndCallStore()
            }}
          >
            拨打电话
          </Button>
          {storeGuideVideoConfigured ? (
            <Button
              className="home-location-btn home-location-btn-secondary"
              size="mini"
              onClick={(event) => {
                event.stopPropagation()
                handleOpenStoreGuide()
              }}
            >
              视频导航
            </Button>
          ) : null}
          <Button
            className="home-location-btn home-location-btn-primary"
            size="mini"
            onClick={(event) => {
              event.stopPropagation()
              void handleOpenLocation()
            }}
          >
            {locationConfigured ? '导航去店' : '待配置'}
          </Button>
        </View>
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
                  <Image className="home-product-cover" src={resolveAssetURL(item.cover_url)} mode="aspectFill" lazyLoad />
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
                      void promptAndCallStore()
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
