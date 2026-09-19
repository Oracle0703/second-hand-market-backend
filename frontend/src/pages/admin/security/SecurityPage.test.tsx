import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, expect, it, vi } from 'vitest'
import { SecurityPage } from './SecurityPage'
import { useAuthStore } from '@/stores/auth-store'

const changePassword = vi.fn()
const navigate = vi.fn()
vi.mock('@/services/api', () => ({ api: {
  adminAccount: async () => ({ data: { data: { account: { id: 9, username: 'test-admin', role: 'ADMIN', status: 'ACTIVE' } } } }),
  adminChangePassword: (...args: unknown[]) => changePassword(...args)
} }))
vi.mock('react-router-dom', async () => ({ ...await vi.importActual<typeof import('react-router-dom')>('react-router-dom'), useNavigate: () => navigate }))

beforeEach(() => {
  changePassword.mockReset()
  navigate.mockReset()
  useAuthStore.getState().clear()
  useAuthStore.getState().setAuth({ accessToken: 'access', refreshToken: 'refresh', user: { id: 9, role: 'ADMIN' } })
})
function setup() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(<QueryClientProvider client={client}><MemoryRouter><SecurityPage /></MemoryRouter></QueryClientProvider>)
}
function fill(confirm = 'ReplacementPass456!') {
  fireEvent.change(screen.getByLabelText('旧密码'), { target: { value: 'ExistingPass123!' } })
  fireEvent.change(screen.getByLabelText('新密码'), { target: { value: 'ReplacementPass456!' } })
  fireEvent.change(screen.getByLabelText('确认新密码'), { target: { value: confirm } })
}
const submit = () => fireEvent.click(screen.getByRole('button', { name: /修改密码并重新登录/ }))

it('validates password confirmation before making a request', async () => {
  setup(); fill('DifferentPass789!'); submit()
  expect(await screen.findByText('两次输入的新密码不一致')).toBeInTheDocument()
  expect(changePassword).not.toHaveBeenCalled()
})

it('prevents duplicate submissions, omits confirmation and clears credentials on success', async () => {
  let finish!: () => void
  changePassword.mockReturnValue(new Promise<void>((resolve) => { finish = resolve }))
  setup(); fill(); submit(); submit()
  await waitFor(() => expect(changePassword).toHaveBeenCalledTimes(1))
  expect(changePassword).toHaveBeenCalledWith({ old_password: 'ExistingPass123!', new_password: 'ReplacementPass456!' })
  expect(screen.getByRole('button', { name: /修改密码并重新登录/ })).toBeDisabled()
  finish()
  await waitFor(() => expect(navigate).toHaveBeenCalledWith('/admin/login', { replace: true }))
  expect(useAuthStore.getState().accessToken).toBe('')
  expect(useAuthStore.getState().refreshToken).toBe('')
  expect(useAuthStore.getState().user).toBeNull()
  expect(screen.getByLabelText('新密码')).toHaveValue('')
})

it('keeps the session on failure and allows retry', async () => {
  changePassword.mockRejectedValueOnce(new Error('旧密码不正确'))
  setup(); fill(); submit()
  expect(await screen.findByText('旧密码不正确')).toBeInTheDocument()
  expect(useAuthStore.getState().accessToken).toBe('access')
  expect(navigate).not.toHaveBeenCalled()
  changePassword.mockResolvedValue({})
  submit()
  await waitFor(() => expect(navigate).toHaveBeenCalledWith('/admin/login', { replace: true }))
  expect(changePassword).toHaveBeenCalledTimes(2)
})

it('does not log out a different identity when an old request finishes', async () => {
  let finish!: () => void
  changePassword.mockReturnValue(new Promise<void>((resolve) => { finish = resolve }))
  setup(); fill(); submit()
  await waitFor(() => expect(changePassword).toHaveBeenCalledTimes(1))
  useAuthStore.getState().setAuth({ accessToken: 'other-access', refreshToken: 'other-refresh', user: { id: 10, role: 'ADMIN' } })
  finish()
  await waitFor(() => expect(screen.getByRole('button', { name: /修改密码并重新登录/ })).not.toBeDisabled())
  expect(useAuthStore.getState().accessToken).toBe('other-access')
  expect(navigate).not.toHaveBeenCalled()
})
