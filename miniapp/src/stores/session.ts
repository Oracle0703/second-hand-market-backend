import { create } from 'zustand'
import Taro from '@tarojs/taro'
import { generateDeviceID } from '../utils/device'

export type BuyerProfile = {
  id: number
  buyer_no: string
  nickname?: string
  avatar_url?: string
  phone?: string
}

type SessionState = {
  accessToken: string
  refreshToken: string
  profile?: BuyerProfile
  deviceID: string
  setSession: (accessToken: string, refreshToken: string, profile?: BuyerProfile) => void
  clearSession: () => void
  setDeviceID: (deviceID: string) => void
}

const STORAGE_KEYS = {
  access: 'buyer_access_token',
  refresh: 'buyer_refresh_token',
  profile: 'buyer_profile',
  deviceID: 'buyer_device_id'
}

const initialDeviceID = (() => {
  const cached = Taro.getStorageSync<string>(STORAGE_KEYS.deviceID)
  if (cached) return cached
  const generated = generateDeviceID()
  Taro.setStorageSync(STORAGE_KEYS.deviceID, generated)
  return generated
})()

const initialAccess = Taro.getStorageSync<string>(STORAGE_KEYS.access) || ''
const initialRefresh = Taro.getStorageSync<string>(STORAGE_KEYS.refresh) || ''
const initialProfile = Taro.getStorageSync<BuyerProfile>(STORAGE_KEYS.profile)

export const useSessionStore = create<SessionState>((set) => ({
  accessToken: initialAccess,
  refreshToken: initialRefresh,
  profile: initialProfile,
  deviceID: initialDeviceID,
  setSession: (accessToken, refreshToken, profile) => {
    Taro.setStorageSync(STORAGE_KEYS.access, accessToken)
    Taro.setStorageSync(STORAGE_KEYS.refresh, refreshToken)
    if (profile) {
      Taro.setStorageSync(STORAGE_KEYS.profile, profile)
    }
    set({ accessToken, refreshToken, profile })
  },
  clearSession: () => {
    Taro.removeStorageSync(STORAGE_KEYS.access)
    Taro.removeStorageSync(STORAGE_KEYS.refresh)
    Taro.removeStorageSync(STORAGE_KEYS.profile)
    set({ accessToken: '', refreshToken: '', profile: undefined })
  },
  setDeviceID: (deviceID) => {
    Taro.setStorageSync(STORAGE_KEYS.deviceID, deviceID)
    set({ deviceID })
  }
}))

export function ensureDeviceID(): string {
  const state = useSessionStore.getState()
  if (state.deviceID) return state.deviceID
  const generated = generateDeviceID()
  state.setDeviceID(generated)
  return generated
}

export function isLoggedIn(): boolean {
  return !!useSessionStore.getState().accessToken
}
