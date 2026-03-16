import { create } from 'zustand'
import { createJSONStorage, persist } from 'zustand/middleware'
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

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      accessToken: '',
      refreshToken: '',
      tokenScope: '',
      user: null,
      setAuth: ({ accessToken, refreshToken, tokenScope = 'full', user }) =>
        set({ accessToken, refreshToken, tokenScope, user }),
      clear: () => set({ accessToken: '', refreshToken: '', tokenScope: '', user: null })
    }),
    {
      name: 'auth-store',
      storage: createJSONStorage(() => localStorage)
    }
  )
)
