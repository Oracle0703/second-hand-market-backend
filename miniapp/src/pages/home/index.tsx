import React from 'react'
import { useQuery } from '@tanstack/react-query'
import Taro from '@tarojs/taro'
import { Text, View } from '@tarojs/components'
import { fetchBuyerProducts } from '../../services/buyer'
import { ProductCard } from '../../components/ProductCard'

export default function HomePage() {
  const query = useQuery({
    queryKey: ['buyer-home-products'],
    queryFn: () => fetchBuyerProducts({ page: 1, page_size: 20, sort: 'latest' })
  })

  return (
    <View className="page">
      <View className="title">今日上新</View>
      {query.isLoading ? <Text>加载中...</Text> : null}
      {query.error ? <Text>{(query.error as Error).message}</Text> : null}
      {(query.data?.items || []).length === 0 && !query.isLoading ? <View className="empty">暂无在售商品</View> : null}
      {(query.data?.items || []).map((item) => (
        <ProductCard key={item.id} product={item} onClick={() => Taro.navigateTo({ url: `/pages/product/detail/index?id=${item.id}` })} />
      ))}
    </View>
  )
}
