import React, { useState } from 'react'
import Taro from '@tarojs/taro'
import { Button, Input, Text, View } from '@tarojs/components'

const KEY = 'buyer_search_history'

export default function SearchPage() {
  const [keyword, setKeyword] = useState('')
  const [history, setHistory] = useState<string[]>(() => Taro.getStorageSync<string[]>(KEY) || [])

  const goSearch = (kw: string) => {
    const trimmed = kw.trim()
    if (!trimmed) return
    const next = [trimmed, ...history.filter((h) => h !== trimmed)].slice(0, 10)
    setHistory(next)
    Taro.setStorageSync(KEY, next)
    Taro.navigateTo({ url: `/pages/product/list/index?keyword=${encodeURIComponent(trimmed)}` })
  }

  return (
    <View className="page">
      <View className="card" style={{ marginBottom: '20rpx' }}>
        <Input
          value={keyword}
          placeholder="搜索商品关键词"
          onInput={(e) => setKeyword(e.detail.value)}
          confirmType="search"
          onConfirm={() => goSearch(keyword)}
        />
        <Button className="btn-primary" size="mini" style={{ marginTop: '12rpx' }} onClick={() => goSearch(keyword)}>
          搜索
        </Button>
      </View>

      <View className="card">
        <View className="title">最近搜索</View>
        {history.length === 0 ? <View className="empty">暂无历史关键词</View> : null}
        {history.map((item) => (
          <View key={item} className="list-item" onClick={() => goSearch(item)}>
            <Text>{item}</Text>
          </View>
        ))}
      </View>
    </View>
  )
}
