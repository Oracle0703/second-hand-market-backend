export type StoreLocation = {
  name: string
  address: string
  latitude: number | null
  longitude: number | null
}

export type StoreGuideVideo = {
  title: string
  url: string
  poster: string
}

export const STORE_LOCATION: StoreLocation = {
  name: '德阳·瑞扬二手家具家电',
  address: '广汉市兴业物资综合市场A区',
  latitude: 30.998248443429098,
  longitude: 104.30505350232124
}

export const STORE_GUIDE_VIDEO: StoreGuideVideo = {
  title: '到店导航视频',
  url: 'https://market.meaningful.ink/assets/videos/store-guide.mp4',
  poster: ''
}

export function hasStoreLocation(location: StoreLocation) {
  return Boolean(
    location.address.trim() &&
    typeof location.latitude === 'number' &&
    Number.isFinite(location.latitude) &&
    typeof location.longitude === 'number' &&
    Number.isFinite(location.longitude)
  )
}

export function hasStoreGuideVideo(video: StoreGuideVideo) {
  return Boolean(video.url.trim())
}
