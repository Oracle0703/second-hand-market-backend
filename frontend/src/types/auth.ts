export type LoginType = 'ADMIN' | 'MERCHANT'

export type AuthUser = {
  id: number
  role: string
  must_change_password?: boolean
  merchant_id?: number
}

export type LoginResponse = {
  access_token: string
  refresh_token: string
  expires_in: number
  token_scope?: 'full' | 'onboarding'
  review_status?: string
  user: AuthUser
}
