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
  hydrated: boolean
  // Identifies this login even if its tokens rotate or the same buyer signs in again.
  sessionVersion: number
  setSession: (accessToken: string, refreshToken: string, profile?: BuyerProfile) => number
  rotateTokens: (accessToken: string, refreshToken: string) => void
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

function storeTokens(accessToken: string, refreshToken: string) {
  Taro.setStorageSync(STORAGE_KEYS.access, accessToken)
  Taro.setStorageSync(STORAGE_KEYS.refresh, refreshToken)
}

export const useSessionStore = create<SessionState>((set, get) => ({
  accessToken: '',
  refreshToken: '',
  profile: undefined,
  deviceID: '',
  hydrated: false,
  sessionVersion: 0,
  setSession: (accessToken, refreshToken, profile) => {
    const sessionVersion = get().sessionVersion + 1
    storeTokens(accessToken, refreshToken)
    if (profile) {
      Taro.setStorageSync(STORAGE_KEYS.profile, profile)
    } else {
      Taro.removeStorageSync(STORAGE_KEYS.profile)
    }
    set({ accessToken, refreshToken, profile, sessionVersion, hydrated: true })
    return sessionVersion
  },
  rotateTokens: (accessToken, refreshToken) => {
    storeTokens(accessToken, refreshToken)
    set({ accessToken, refreshToken })
  },
  clearSession: () => {
    Taro.removeStorageSync(STORAGE_KEYS.access)
    Taro.removeStorageSync(STORAGE_KEYS.refresh)
    Taro.removeStorageSync(STORAGE_KEYS.profile)
    set((state) => ({ accessToken: '', refreshToken: '', profile: undefined, sessionVersion: state.sessionVersion + 1, hydrated: true }))
  },
  setDeviceID: (deviceID) => {
    Taro.setStorageSync(STORAGE_KEYS.deviceID, deviceID)
    set({ deviceID, hydrated: true })
  }
}))

export function hydrateSessionStore(): void {
  if (!useSessionStore.getState().hydrated) {
    useSessionStore.setState({ ...loadSessionSnapshot(), hydrated: true })
  }
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
