import { useDidShow } from '@tarojs/taro'
import { trackPageView } from '../services/analytics'

export function usePageExposure(page: string, productId?: number) {
  useDidShow(() => trackPageView(page, productId))
}
