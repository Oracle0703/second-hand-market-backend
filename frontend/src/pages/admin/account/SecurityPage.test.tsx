import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { SecurityPage } from './SecurityPage'
import { useAuthStore } from '@/stores/auth-store'

const mockNavigate = vi.fn()
const mockChangePassword = vi.fn()

vi.mock('@ant-design/pro-components', () => import('@/test/pro-components-stub'))

vi.mock('@/services/api', () => ({
  api: {
    adminChangePassword: (...args: unknown[]) => mockChangePassword(...args)
  }
}))

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom')
  return { ...actual, useNavigate: () => mockNavigate }
})

function renderPage() {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <SecurityPage />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('SecurityPage', () => {
  beforeEach(() => {
    mockNavigate.mockReset()
    mockChangePassword.mockReset()
    useAuthStore.getState().setAuth({
      accessToken: 'old-access',
      refreshToken: 'old-refresh',
      tokenScope: 'full',
      user: { id: 1, role: 'ADMIN' }
    })
  })

  it('clears tokens and redirects to admin login after changing password', async () => {
    mockChangePassword.mockResolvedValue({ data: { data: { success: true } } })
    renderPage()

    fireEvent.change(screen.getByLabelText('当前密码'), { target: { value: 'CurrentAdmin@2026' } })
    fireEvent.change(screen.getByLabelText('新密码'), { target: { value: 'DifferentAdmin@2026' } })
    fireEvent.change(screen.getByLabelText('确认新密码'), { target: { value: 'DifferentAdmin@2026' } })
    fireEvent.click(screen.getByRole('button', { name: /保\s*存\s*密\s*码/ }))

    await waitFor(() => {
      expect(mockChangePassword).toHaveBeenCalledWith({
        current_password: 'CurrentAdmin@2026',
        new_password: 'DifferentAdmin@2026'
      })
    })
    expect(useAuthStore.getState().accessToken).toBe('')
    expect(useAuthStore.getState().refreshToken).toBe('')
    expect(mockNavigate).toHaveBeenCalledWith('/admin/login', { replace: true })
  })
})
