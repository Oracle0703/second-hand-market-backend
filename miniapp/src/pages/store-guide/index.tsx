import React from 'react'
import { Text, Video, View } from '@tarojs/components'
import { hasStoreGuideVideo, STORE_GUIDE_VIDEO } from '../../constants/store'

export default function StoreGuidePage() {
  const ready = hasStoreGuideVideo(STORE_GUIDE_VIDEO)

  return (
    <View className="page store-guide-page">
      <View className="store-guide-card">
        <Text className="store-guide-kicker">到店导航</Text>
        <Text className="store-guide-title">{STORE_GUIDE_VIDEO.title}</Text>
        <Text className="store-guide-desc">
          {ready ? '点击播放即可查看从当前位置前往门店的视频路线。' : '暂未配置导航视频，补充视频地址后即可播放。'}
        </Text>
      </View>

      {ready ? (
        <Video
          className="store-guide-video"
          src={STORE_GUIDE_VIDEO.url}
          poster={STORE_GUIDE_VIDEO.poster}
          controls
          autoplay={false}
          showCenterPlayBtn
          showPlayBtn
          objectFit="contain"
        />
      ) : (
        <View className="store-guide-empty">
          <Text className="store-guide-empty-title">暂无导航视频</Text>
          <Text className="store-guide-empty-desc">请在 `miniapp/src/constants/store.ts` 中填写视频地址。</Text>
        </View>
      )}
    </View>
  )
}
