import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { LoginPage } from './LoginPage'
import { useAuthStore } from '../stores/auth-store'

const mockNavigate = vi.fn()
const mockLogin = vi.fn()

vi.mock('../services/api', () => ({
  api: {
    login: (...args: unknown[]) => mockLogin(...args)
  }
}))

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom')
  return {
    ...actual,
    useNavigate: () => mockNavigate
  }
})

describe('LoginPage', () => {
  beforeEach(() => {
    mockNavigate.mockReset()
    mockLogin.mockReset()
    useAuthStore.getState().clear()
  })

  it('redirects restricted merchant login to /register/status', async () => {
    mockLogin.mockResolvedValue({
      data: {
        code: 0,
        message: 'OK',
        request_id: 'req_1',
        data: {
          access_token: 'a',
          refresh_token: 'r',
          expires_in: 7200,
          token_scope: 'onboarding',
          review_status: 'PENDING',
          user: { id: 1, role: 'OWNER', merchant_id: 100 }
        }
      }
    })

    render(
      <MemoryRouter>
        <LoginPage />
      </MemoryRouter>
    )

    fireEvent.change(screen.getByLabelText('账号'), { target: { value: 'merchant_user' } })
    fireEvent.change(screen.getByLabelText('密码'), { target: { value: 'Passw0rd!2026' } })
    fireEvent.click(screen.getByRole('button', { name: '登录' }))

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/register/status')
    })
    expect(useAuthStore.getState().tokenScope).toBe('onboarding')
  })
})
