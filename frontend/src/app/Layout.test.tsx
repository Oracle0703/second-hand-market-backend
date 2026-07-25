import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { Layout } from './Layout'
import { useAuthStore } from '../stores/auth-store'

const mockNavigate = vi.fn()
const mockPost = vi.fn()

vi.mock('@ant-design/pro-components', () => import('@/test/pro-components-stub'))

vi.mock('../services/http', () => ({
  http: {
    post: (...args: unknown[]) => mockPost(...args)
  }
}))

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom')
  return {
    ...actual,
    useNavigate: () => mockNavigate
  }
})

function renderLayout(initialPath = '/merchant/dashboard') {
  return render(
    <MemoryRouter initialEntries={[initialPath]}>
      <Routes>
        <Route element={<Layout />}>
          <Route path={initialPath} element={<div>page-body</div>} />
        </Route>
      </Routes>
    </MemoryRouter>
  )
}

function setMerchantAuth() {
  useAuthStore.getState().setAuth({
    accessToken: 'merchant-access',
    refreshToken: 'merchant-refresh',
    tokenScope: 'full',
    user: { id: 10, role: 'OWNER', merchant_id: 1001 }
  })
}

function setAdminAuth() {
  useAuthStore.getState().setAuth({
    accessToken: 'admin-access',
    refreshToken: 'admin-refresh',
    tokenScope: 'full',
    user: { id: 1, role: 'ADMIN' }
  })
}

describe('Layout logout', () => {
  beforeEach(() => {
    mockNavigate.mockReset()
    mockPost.mockReset()
    useAuthStore.getState().clear()
  })

  it('revokes the merchant session before clearing local credentials', async () => {
    setMerchantAuth()

    let resolveLogout!: (value: unknown) => void
    mockPost.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveLogout = resolve
        })
    )

    renderLayout()

    const logoutButton = screen.getByRole('button', { name: /退\s*出/ })
    fireEvent.click(logoutButton)

    await waitFor(() => {
      expect(mockPost).toHaveBeenCalledWith('/auth/logout', {})
    })
    expect(mockPost).toHaveBeenCalledTimes(1)
    expect(useAuthStore.getState().accessToken).toBe('merchant-access')
    expect(useAuthStore.getState().refreshToken).toBe('merchant-refresh')
    expect(logoutButton.hasAttribute('disabled') || logoutButton.getAttribute('aria-disabled') === 'true').toBe(true)
    expect(logoutButton.classList.contains('ant-btn-loading')).toBe(true)
    expect(mockNavigate).not.toHaveBeenCalled()

    resolveLogout({
      data: {
        code: 0,
        message: 'OK',
        request_id: 'req_logout',
        data: { success: true }
      }
    })

    await waitFor(() => {
      expect(useAuthStore.getState().accessToken).toBe('')
      expect(useAuthStore.getState().refreshToken).toBe('')
      expect(useAuthStore.getState().tokenScope).toBe('')
      expect(useAuthStore.getState().user).toBeNull()
      expect(mockNavigate).toHaveBeenCalledWith('/login')
    })
  })

  it('still clears admin credentials and navigates when logout fails', async () => {
    setAdminAuth()
    mockPost.mockRejectedValue(new Error('network down'))

    renderLayout('/admin/merchants/reviews')

    fireEvent.click(screen.getByRole('button', { name: /退\s*出/ }))

    await waitFor(() => {
      expect(mockPost).toHaveBeenCalledWith('/auth/logout', {})
      expect(useAuthStore.getState().accessToken).toBe('')
      expect(useAuthStore.getState().refreshToken).toBe('')
      expect(useAuthStore.getState().tokenScope).toBe('')
      expect(useAuthStore.getState().user).toBeNull()
      expect(mockNavigate).toHaveBeenCalledWith('/admin/login')
    })
  })

  it('ignores duplicate logout clicks while a request is in flight', async () => {
    setMerchantAuth()

    let resolveLogout!: (value: unknown) => void
    mockPost.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveLogout = resolve
        })
    )

    renderLayout()

    const logoutButton = screen.getByRole('button', { name: /退\s*出/ })
    fireEvent.click(logoutButton)
    fireEvent.click(logoutButton)

    await waitFor(() => {
      expect(mockPost).toHaveBeenCalledTimes(1)
    })

    resolveLogout({
      data: {
        code: 0,
        message: 'OK',
        request_id: 'req_logout',
        data: { success: true }
      }
    })

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/login')
    })
    expect(mockPost).toHaveBeenCalledTimes(1)
  })
})
