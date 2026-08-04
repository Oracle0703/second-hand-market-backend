import React, { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@/libs/react-query'
import Taro, { useDidShow, useReachBottom } from '@tarojs/taro'
import { Button, Image, Text, View } from '@tarojs/components'
import { BuyerProduct, fetchBuyerCategories, fetchBuyerProducts } from '../../services/buyer'
import { promptAndCallStore } from '../../utils/contact'
import { centToYuanText } from '../../utils/price'
import { resolveAssetURL } from '../../utils/url'

const PAGE_SIZE = 10

function mergeProducts(current: BuyerProduct[], incoming: BuyerProduct[]): BuyerProduct[] {
  const seen = new Set<number>()
  return [...current, ...incoming].filter((item) => {
    if (seen.has(item.id)) {
      return false
    }
    seen.add(item.id)
    return true
  })
}

export default function CategoryPage() {
  const level1Query = useQuery({ queryKey: ['buyer-categories-l1'], queryFn: () => fetchBuyerCategories({ level: 1 }) })
  const [activeParentID, setActiveParentID] = useState<number | undefined>(undefined)
  const [selectedChildID, setSelectedChildID] = useState<number | undefined>(undefined)
  const [items, setItems] = useState<BuyerProduct[]>([])
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const [loadingProducts, setLoadingProducts] = useState(false)
  const [loadingMore, setLoadingMore] = useState(false)
  const [productError, setProductError] = useState('')
  const loadingProductsRef = useRef(false)
  const requestKeyRef = useRef('')
  const didShowRef = useRef(false)

  const topCategories = level1Query.data?.items || []
  const parentID = activeParentID || topCategories[0]?.id

  const level2Query = useQuery({
    queryKey: ['buyer-categories-l2', parentID],
    queryFn: () => fetchBuyerCategories({ level: 2, parent_id: parentID }),
    enabled: !!parentID
  })

  const childCategories = level2Query.data?.items || []
  const selectedChild = useMemo(
    () => childCategories.find((cat) => cat.id === selectedChildID),
    [childCategories, selectedChildID]
  )
  const hasMore = items.length < total
  const showingProducts = !!selectedChild

  const loadProducts = async (targetPage: number, categoryID: number, replace = false) => {
    if (!categoryID || (loadingProductsRef.current && !replace)) {
      return
    }

    const requestKey = `${categoryID}:${targetPage}:${replace ? 'replace' : 'append'}`
    requestKeyRef.current = requestKey
    loadingProductsRef.current = true
    setProductError('')

    if (replace) {
      setLoadingProducts(true)
    } else {
      setLoadingMore(true)
    }

    try {
      const result = await fetchBuyerProducts({
        page: targetPage,
        page_size: PAGE_SIZE,
        sort: 'latest',
        category_id: categoryID
      })

      if (requestKeyRef.current !== requestKey) {
        return
      }

      const nextItems = result.items || []
      setPage(result.page || targetPage)
      setTotal(result.total || 0)
      setItems((prev) => (replace ? nextItems : mergeProducts(prev, nextItems)))
    } catch (err) {
      if (requestKeyRef.current === requestKey) {
        setProductError(err instanceof Error ? err.message : '商品加载失败')
      }
    } finally {
      if (requestKeyRef.current === requestKey) {
        loadingProductsRef.current = false
        setLoadingProducts(false)
        setLoadingMore(false)
      }
    }
  }

  useEffect(() => {
    if (!activeParentID && topCategories.length > 0) {
      setActiveParentID(topCategories[0].id)
    }
  }, [activeParentID, topCategories])

  useEffect(() => {
    setSelectedChildID(undefined)
    setItems([])
    setPage(1)
    setTotal(0)
    setProductError('')
  }, [parentID])

  useDidShow(() => {
    if (!didShowRef.current) {
      didShowRef.current = true
      return
    }
    if (selectedChild?.id) {
      void loadProducts(1, selectedChild.id, true)
    }
  })

  useReachBottom(() => {
    if (selectedChild?.id && !loadingProductsRef.current && hasMore) {
      void loadProducts(page + 1, selectedChild.id)
    }
  })

  return (
    <View className="page category-page">
      <View className="category-shell">
        <View className="category-sidebar">
          <View className="category-sidebar-head">
            <Text className="category-sidebar-title">一级分类</Text>
          </View>

          {topCategories.map((cat) => {
            const isActive = cat.id === parentID

            return (
              <View
                key={cat.id}
                className={`category-tab ${isActive ? 'category-tab-active' : ''}`}
                onClick={() => setActiveParentID(cat.id)}
              >
                <Text className={`category-tab-text ${isActive ? 'category-tab-text-active' : ''}`}>{cat.name}</Text>
              </View>
            )
          })}
        </View>

        <View className="category-content">
          {!showingProducts ? (
            <View>
              <View className="category-content-head">
                <Text className="title" style={{ marginBottom: 0 }}>
                  {topCategories.find((cat) => cat.id === parentID)?.name || '分类导航'}
                </Text>
                <Text className="category-content-meta">
                  {level2Query.isLoading ? '加载中...' : `${childCategories.length} 个分类`}
                </Text>
              </View>

              {level1Query.error ? <View className="empty">{(level1Query.error as Error).message}</View> : null}
              {level2Query.error ? <View className="empty">{(level2Query.error as Error).message}</View> : null}

              <View className="category-list">
                {childCategories.map((cat) => (
                  <View key={cat.id} className="category-list-row">
                    <Text className="category-list-name">{cat.name}</Text>
                    <Text
                      className="category-list-link"
                      onClick={() => {
                        setSelectedChildID(cat.id)
                        void loadProducts(1, cat.id, true)
                      }}
                    >
                      查看更多
                    </Text>
                  </View>
                ))}
              </View>

              {!level2Query.isLoading && childCategories.length === 0 ? <View className="empty">暂无二级分类</View> : null}
            </View>
          ) : (
            <View>
              <View className="category-content-head">
                <View>
                  <Text className="title" style={{ marginBottom: '6rpx', display: 'block' }}>
                    {selectedChild?.name}
                  </Text>
                  <Text className="category-back-link" onClick={() => setSelectedChildID(undefined)}>
                    返回分类列表
                  </Text>
                </View>
                <Text className="category-content-meta">{loadingProducts ? '加载中...' : `${total} 件商品`}</Text>
              </View>

              {productError ? <View className="empty">{productError}</View> : null}
              {loadingProducts ? <View className="empty">加载中...</View> : null}
              {!loadingProducts && !productError && items.length === 0 ? <View className="empty">该分类下暂无在售商品</View> : null}

              <View className="category-product-list">
                {items.map((item) => {
                  const originalPriceCent = item.original_price_cent ?? item.price_cent

                  return (
                    <View key={item.id} className="category-product-row">
                      <View className="category-product-cover-wrap" onClick={() => Taro.navigateTo({ url: `/pages/product/detail/index?id=${item.id}` })}>
                        {item.cover_url ? (
                          <Image className="category-product-cover" src={resolveAssetURL(item.cover_url)} mode="aspectFill" lazyLoad />
                        ) : (
                          <View className="category-product-placeholder">
                            <Text className="category-product-placeholder-text">暂无图片</Text>
                          </View>
                        )}
                      </View>

                      <View className="category-product-body">
                        <Text className="category-product-title" onClick={() => Taro.navigateTo({ url: `/pages/product/detail/index?id=${item.id}` })}>
                          {item.title}
                        </Text>
                        <View className="category-product-price-row">
                          <Text className="category-product-price">售价 ¥{centToYuanText(item.price_cent)}</Text>
                          <Text className="category-product-original">原价 ¥{centToYuanText(originalPriceCent)}</Text>
                        </View>
                        <View className="category-product-action">
                          <Button
                            className="category-want-btn"
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

              {!loadingProducts && !productError && items.length > 0 ? (
                <View className="home-loadmore">
                  <Text>{loadingMore ? '正在加载更多商品...' : hasMore ? '下拉到底部继续加载' : '已经到底了'}</Text>
                </View>
              ) : null}
            </View>
          )}
        </View>
      </View>
    </View>
  )
}
