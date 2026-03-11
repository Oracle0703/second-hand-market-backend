import React, { useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import Taro, { useDidShow, useRouter } from '@tarojs/taro'
import { Button, Input, Text, Textarea, View } from '@tarojs/components'
import { createIntent, fetchBuyerProductDetail } from '../../../services/buyer'
import { requireLoginFor } from '../../../hooks/useRequireLogin'

export default function IntentCreatePage() {
  const router = useRouter()
  const productID = Number(router.params.product_id || 0)
  const [contactName, setContactName] = useState('')
  const [contactPhone, setContactPhone] = useState('')
  const [contactWechat, setContactWechat] = useState('')
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')

  useDidShow(() => {
    const current = `/pages/intent/create/index?product_id=${productID}`
    requireLoginFor(current)
  })

  const detail = useQuery({
    queryKey: ['intent-product-detail', productID],
    queryFn: () => fetchBuyerProductDetail(productID),
    enabled: productID > 0
  })

  const submitMutation = useMutation({
    mutationFn: async () => {
      return createIntent({
        product_id: productID,
        contact_name: contactName || undefined,
        contact_phone: contactPhone || undefined,
        contact_wechat: contactWechat || undefined,
        message: message || undefined
      })
    },
    onSuccess: () => {
      Taro.showToast({ title: '提交成功', icon: 'success' })
      Taro.redirectTo({ url: '/pages/intent/list/index' })
    },
    onError: (e) => {
      setError((e as Error).message)
    }
  })

  const product = detail.data?.product
  const submit = () => {
    if (!contactPhone.trim() && !contactWechat.trim()) {
      setError('手机号或微信号至少填写一项')
      return
    }
    if (!product?.can_submit_intent) {
      setError('当前商品不可提交意向')
      return
    }
    setError('')
    submitMutation.mutate()
  }

  return (
    <View className="page">
      <View className="card" style={{ marginBottom: '16rpx' }}>
        <View className="title">意向商品</View>
        <Text>{product?.title || '加载中...'}</Text>
      </View>

      <View className="card">
        <View className="list-item">
          <Text>联系人</Text>
          <Input value={contactName} onInput={(e) => setContactName(e.detail.value)} placeholder="选填" />
        </View>
        <View className="list-item">
          <Text>手机号</Text>
          <Input value={contactPhone} onInput={(e) => setContactPhone(e.detail.value)} placeholder="手机号或微信号至少填一项" />
        </View>
        <View className="list-item">
          <Text>微信号</Text>
          <Input value={contactWechat} onInput={(e) => setContactWechat(e.detail.value)} placeholder="手机号或微信号至少填一项" />
        </View>
        <View className="list-item">
          <Text>留言</Text>
          <Textarea value={message} onInput={(e) => setMessage(e.detail.value)} placeholder="选填" maxlength={500} />
        </View>
      </View>

      {error ? <View style={{ color: '#b63732', marginTop: '12rpx' }}>{error}</View> : null}
      <Button className={product?.can_submit_intent ? 'btn-primary' : 'btn-disabled'} style={{ marginTop: '16rpx' }} onClick={submit} disabled={!product?.can_submit_intent}>
        提交意向
      </Button>
    </View>
  )
}
