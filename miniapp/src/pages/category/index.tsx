import React, { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import Taro from '@tarojs/taro'
import { Text, View } from '@tarojs/components'
import { fetchBuyerCategories } from '../../services/buyer'

export default function CategoryPage() {
  const level1Query = useQuery({ queryKey: ['buyer-categories-l1'], queryFn: () => fetchBuyerCategories({ level: 1 }) })
  const [activeParentID, setActiveParentID] = useState<number | undefined>(undefined)

  const parentID = activeParentID || level1Query.data?.items?.[0]?.id
  const level2Query = useQuery({
    queryKey: ['buyer-categories-l2', parentID],
    queryFn: () => fetchBuyerCategories({ level: 2, parent_id: parentID }),
    enabled: !!parentID
  })

  const topCategories = level1Query.data?.items || []
  const childCategories = level2Query.data?.items || []

  useMemo(() => {
    if (!activeParentID && topCategories.length > 0) {
      setActiveParentID(topCategories[0].id)
    }
  }, [activeParentID, topCategories])

  return (
    <View className="page">
      <View className="title">分类导航</View>
      <View className="card" style={{ marginBottom: '16rpx' }}>
        {topCategories.map((cat) => (
          <Text
            key={cat.id}
            className="status-badge"
            style={{ marginRight: '12rpx', marginBottom: '8rpx', background: cat.id === parentID ? '#1d5a4a' : '#eef4f1', color: cat.id === parentID ? '#fff' : '#1d5a4a' }}
            onClick={() => setActiveParentID(cat.id)}
          >
            {cat.name}
          </Text>
        ))}
      </View>

      <View className="card">
        {(childCategories || []).map((cat) => (
          <View
            key={cat.id}
            className="list-item"
            onClick={() => Taro.navigateTo({ url: `/pages/product/list/index?category_id=${cat.id}&category_name=${encodeURIComponent(cat.name)}` })}
          >
            <Text>{cat.name}</Text>
          </View>
        ))}
        {childCategories.length === 0 ? <View className="empty">暂无二级分类</View> : null}
      </View>
    </View>
  )
}
