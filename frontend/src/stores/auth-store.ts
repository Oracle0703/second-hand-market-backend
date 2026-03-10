import { create } from 'zustand'
import type { AuthUser } from '../types/auth'

type AuthState = {
  accessToken: string
  refreshToken: string
  tokenScope: 'full' | 'onboarding' | ''
  user: AuthUser | null
  setAuth: (payload: {
    accessToken: string
    refreshToken: string
    tokenScope?: 'full' | 'onboarding'
    user: AuthUser
  }) => void
  clear: () => void
}

export const useAuthStore = create<AuthState>((set) => ({
  accessToken: '',
  refreshToken: '',
  tokenScope: '',
  user: null,
  setAuth: ({ accessToken, refreshToken, tokenScope = 'full', user }) =>
    set({ accessToken, refreshToken, tokenScope, user }),
  clear: () => set({ accessToken: '', refreshToken: '', tokenScope: '', user: null })
}))
