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

type SessionSnapshot = Pick<SessionState, 'accessToken' | 'refreshToken' | 'profile' | 'deviceID'>

let hydrated = false

function readStorageSync<T>(key: string): T | undefined {
  try {
    return Taro.getStorageSync<T>(key)
  } catch {
    return undefined
  }
}

function loadSessionSnapshot(): SessionSnapshot {
  return {
    accessToken: readStorageSync<string>(STORAGE_KEYS.access) || '',
    refreshToken: readStorageSync<string>(STORAGE_KEYS.refresh) || '',
    profile: readStorageSync<BuyerProfile>(STORAGE_KEYS.profile),
    deviceID: readStorageSync<string>(STORAGE_KEYS.deviceID) || ''
  }
}

export const useSessionStore = create<SessionState>((set) => ({
  accessToken: '',
  refreshToken: '',
  profile: undefined,
  deviceID: '',
  setSession: (accessToken, refreshToken, profile) => {
    hydrated = true
    Taro.setStorageSync(STORAGE_KEYS.access, accessToken)
    Taro.setStorageSync(STORAGE_KEYS.refresh, refreshToken)
    if (profile) {
      Taro.setStorageSync(STORAGE_KEYS.profile, profile)
    }
    set({ accessToken, refreshToken, profile })
  },
  clearSession: () => {
    hydrated = true
    Taro.removeStorageSync(STORAGE_KEYS.access)
    Taro.removeStorageSync(STORAGE_KEYS.refresh)
    Taro.removeStorageSync(STORAGE_KEYS.profile)
    set({ accessToken: '', refreshToken: '', profile: undefined })
  },
  setDeviceID: (deviceID) => {
    hydrated = true
    Taro.setStorageSync(STORAGE_KEYS.deviceID, deviceID)
    set({ deviceID })
  }
}))

export function hydrateSessionStore(): void {
  if (hydrated) {
    return
  }

  hydrated = true
  useSessionStore.setState(loadSessionSnapshot())
}

export function ensureDeviceID(): string {
  hydrateSessionStore()
  const state = useSessionStore.getState()
  if (state.deviceID) return state.deviceID
  const generated = generateDeviceID()
  state.setDeviceID(generated)
  return generated
}

export function isLoggedIn(): boolean {
  hydrateSessionStore()
  return !!useSessionStore.getState().accessToken
}
