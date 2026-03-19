import React from 'react'
import { Image, Text, View } from '@tarojs/components'
import { BuyerProduct } from '../services/buyer'
import { centToYuanText } from '../utils/price'

type Props = {
  product: BuyerProduct
  onClick?: () => void
}

export const ProductCard: React.FC<Props> = ({ product, onClick }) => {
  const originalPriceCent = product.original_price_cent ?? product.price_cent

  return (
    <View className="card list-item" onClick={onClick}>
      {product.cover_url ? <Image src={product.cover_url} mode="aspectFill" style={{ width: '100%', height: '220rpx', borderRadius: '12rpx' }} /> : null}
      <View style={{ marginTop: '12rpx' }}>
        <Text>{product.title}</Text>
      </View>
      <View style={{ marginTop: '8rpx' }}>
        <View style={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap' }}>
          <Text style={{ color: '#d24b2f', fontWeight: 600 }}>售价 ¥{centToYuanText(product.price_cent)}</Text>
          <Text style={{ marginLeft: '12rpx', color: '#8a9691', textDecoration: 'line-through' }}>原价 ¥{centToYuanText(originalPriceCent)}</Text>
        </View>
        <View style={{ marginTop: '8rpx', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <Text style={{ color: '#6f7c77' }}>仅剩 {product.stock} 件</Text>
          <Text className="status-badge">{product.status}</Text>
        </View>
      </View>
    </View>
  )
}
