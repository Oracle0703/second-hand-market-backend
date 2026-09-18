import { create } from 'zustand'
import { createJSONStorage, persist } from 'zustand/middleware'
import type { AuthUser } from '../types/auth'

type AuthState = {
  sessionVersion: number
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

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      sessionVersion: 0,
      accessToken: '',
      refreshToken: '',
      tokenScope: '',
      user: null,
      setAuth: ({ accessToken, refreshToken, tokenScope = 'full', user }) =>
        set((state) => ({
          accessToken, refreshToken, tokenScope, user,
          sessionVersion: state.sessionVersion + (state.user?.id !== user.id || state.user?.role !== user.role || state.user?.merchant_id !== user.merchant_id ? 1 : 0)
        })),
      clear: () => set((state) => ({ accessToken: '', refreshToken: '', tokenScope: '', user: null, sessionVersion: state.sessionVersion + 1 }))
    }),
    {
      name: 'auth-store',
      partialize: ({ sessionVersion: _sessionVersion, ...state }) => state,
      storage: createJSONStorage(() => localStorage)
    }
  )
)
