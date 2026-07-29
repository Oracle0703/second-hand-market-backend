import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuthStore } from '../stores/auth-store'
import { Layout } from './Layout'

const mockNavigate = vi.fn()
const mockPost = vi.fn()

vi.mock('../services/http', () => ({
  http: {
    post: (...args: unknown[]) => mockPost(...args)
  }
}))

vi.mock('@ant-design/pro-components', () => ({
  ProLayout: ({
    actionsRender,
    children
  }: {
    actionsRender?: () => ReactNode[]
    children?: ReactNode
  }) => (
    <div>
      {actionsRender?.()}
      {children}
    </div>
  )
}))

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom')
  return {
    ...actual,
    useNavigate: () => mockNavigate
  }
})

function setAuthenticatedUser(role: string) {
  useAuthStore.getState().setAuth({
    accessToken: 'access-token',
    refreshToken: 'refresh-token',
    user: {
      id: 1,
      role,
      ...(role.includes('ADMIN') ? {} : { merchant_id: 1001 })
    }
  })
}

function renderLayout() {
  return render(
    <MemoryRouter>
      <Layout />
    </MemoryRouter>
  )
}

describe('Layout logout', () => {
  beforeEach(() => {
    mockNavigate.mockReset()
    mockPost.mockReset()
    useAuthStore.getState().clear()
    localStorage.clear()
  })

  it.each([
    ['administrator', 'SUPER_ADMIN'],
    ['merchant', 'OWNER']
  ])('revokes the %s session before clearing local credentials', async (_label, role) => {
    setAuthenticatedUser(role)
    mockPost.mockResolvedValue({ data: { code: 0 } })

    renderLayout()
    fireEvent.click(screen.getByRole('button', { name: /退出/ }))

    await waitFor(() => {
      expect(mockPost).toHaveBeenCalledTimes(1)
      expect(mockPost).toHaveBeenCalledWith('/auth/logout')
      expect(mockNavigate).toHaveBeenCalledWith('/login')
    })
    expect(useAuthStore.getState().accessToken).toBe('')
    expect(useAuthStore.getState().refreshToken).toBe('')
    expect(useAuthStore.getState().user).toBeNull()
  })

  it('finishes local logout when the server request fails', async () => {
    setAuthenticatedUser('OWNER')
    mockPost.mockRejectedValue(new Error('server unavailable'))

    renderLayout()
    fireEvent.click(screen.getByRole('button', { name: /退出/ }))

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/login')
    })
    expect(mockPost).toHaveBeenCalledTimes(1)
    expect(useAuthStore.getState().accessToken).toBe('')
    expect(useAuthStore.getState().user).toBeNull()
  })

  it('allows only one logout flow while the server request is pending', async () => {
    setAuthenticatedUser('OWNER')
    let resolveLogout: (() => void) | undefined
    mockPost.mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          resolveLogout = resolve
        })
    )

    renderLayout()
    const logoutButton = screen.getByRole('button', { name: /退出/ })
    fireEvent.click(logoutButton)
    fireEvent.click(logoutButton)

    expect(mockPost).toHaveBeenCalledTimes(1)
    expect(mockNavigate).not.toHaveBeenCalled()
    expect(useAuthStore.getState().accessToken).toBe('access-token')
    expect((logoutButton as HTMLButtonElement).disabled).toBe(true)

    resolveLogout?.()
    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledTimes(1)
      expect(useAuthStore.getState().accessToken).toBe('')
    })
  })

  it('does not throw when local credentials are already absent', async () => {
    mockPost.mockRejectedValue(new Error('unauthorized'))

    renderLayout()
    fireEvent.click(screen.getByRole('button', { name: /退出/ }))

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/login')
    })
    expect(mockPost).toHaveBeenCalledTimes(1)
    expect(useAuthStore.getState().accessToken).toBe('')
    expect(useAuthStore.getState().user).toBeNull()
  })
})
