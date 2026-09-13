import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it } from 'vitest'
import { RequireAuth } from './guards'
import { useAuthStore } from '../stores/auth-store'

describe('RequireAuth', () => {
  beforeEach(() => {
    useAuthStore.getState().clear()
  })

  it('redirects onboarding merchant from full-scope route to register status', () => {
    useAuthStore.getState().setAuth({
      accessToken: 'token',
      refreshToken: 'refresh',
      tokenScope: 'onboarding',
      user: { id: 1, role: 'OWNER', merchant_id: 1001 }
    })

    render(
      <MemoryRouter initialEntries={['/merchant/products']}>
        <Routes>
          <Route element={<RequireAuth role="MERCHANT" scope="full" />}>
            <Route path="/merchant/products" element={<div>products-page</div>} />
          </Route>
          <Route path="/register/status" element={<div>register-status-page</div>} />
        </Routes>
      </MemoryRouter>
    )

    expect(screen.queryByText('products-page')).toBeNull()
    expect(screen.getByText('register-status-page')).toBeTruthy()
  })
  it('requires an initial password change before entering business pages', () => {
    useAuthStore.getState().setAuth({
      accessToken: 'token', refreshToken: 'refresh', tokenScope: 'full',
      user: { id: 1, role: 'OWNER', merchant_id: 1001, must_change_password: true }
    })
    render(
      <MemoryRouter initialEntries={['/merchant/products']}>
        <Routes>
          <Route element={<RequireAuth role="MERCHANT" scope="full" />}>
            <Route path="/merchant/products" element={<div>business-page</div>} />
            <Route path="/merchant/account" element={<div>change-password-page</div>} />
          </Route>
        </Routes>
      </MemoryRouter>
    )
    expect(screen.queryByText('business-page')).toBeNull()
    expect(screen.getByText('change-password-page')).toBeTruthy()
  })

})
