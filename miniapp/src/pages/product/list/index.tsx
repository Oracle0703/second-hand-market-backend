import React, { useMemo, useState } from 'react'
import { useQuery } from '@/libs/react-query'
import Taro, { useRouter } from '@tarojs/taro'
import { Picker, Text, View } from '@tarojs/components'
import { fetchBuyerProducts } from '../../../services/buyer'
import { ProductCard } from '../../../components/ProductCard'

const SORT_OPTIONS = [
  { label: '最新', value: 'latest' },
  { label: '价格升序', value: 'price_asc' },
  { label: '价格降序', value: 'price_desc' }
]

export default function ProductListPage() {
  const router = useRouter()
  const keyword = router.params.keyword || ''
  const categoryID = router.params.category_id || ''
  const [sortIndex, setSortIndex] = useState(0)

  const params = useMemo(
    () => ({
      page: 1,
      page_size: 20,
      keyword,
      category_id: categoryID,
      sort: SORT_OPTIONS[sortIndex].value
    }),
    [categoryID, keyword, sortIndex]
  )

  const query = useQuery({ queryKey: ['buyer-product-list', params], queryFn: () => fetchBuyerProducts(params) })

  return (
    <View className="page">
      <View className="card" style={{ marginBottom: '16rpx' }}>
        <Text style={{ marginRight: '12rpx' }}>排序：</Text>
        <Picker mode="selector" range={SORT_OPTIONS.map((it) => it.label)} onChange={(e) => setSortIndex(Number(e.detail.value))}>
          <Text className="status-badge">{SORT_OPTIONS[sortIndex].label}</Text>
        </Picker>
      </View>

      {query.isLoading ? <Text>加载中...</Text> : null}
      {query.error ? <Text>{(query.error as Error).message}</Text> : null}
      {(query.data?.items || []).length === 0 && !query.isLoading ? <View className="empty">暂无匹配商品</View> : null}
      {(query.data?.items || []).map((item) => (
        <ProductCard key={item.id} product={item} onClick={() => Taro.navigateTo({ url: `/pages/product/detail/index?id=${item.id}` })} />
      ))}
    </View>
  )
}
