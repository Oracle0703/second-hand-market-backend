import { act, render, screen, waitFor } from '@testing-library/react'
import { useQuery } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SessionQueryProvider } from './SessionQueryProvider'
import { useAuthStore } from '../stores/auth-store'

function login(id: number) {
  useAuthStore.getState().setAuth({ accessToken: `token-${id}`, refreshToken: `refresh-${id}`, user: { id, role: 'MERCHANT', merchant_id: id } })
}

afterEach(() => { act(() => useAuthStore.getState().clear()) })

describe('session query isolation', () => {
  it('does not reuse fresh categories when switching merchants, and keeps cache on token refresh', async () => {
    login(1)
    const load = vi.fn(async () => `merchant-${useAuthStore.getState().user?.id}`)
    function Categories() {
      const { data } = useQuery({ queryKey: ['categories'], queryFn: load, staleTime: 300000 })
      return <div>{data}</div>
    }
    render(<SessionQueryProvider><Categories /></SessionQueryProvider>)
    await screen.findByText('merchant-1')
    act(() => login(2))
    expect(screen.queryByText('merchant-1')).toBeNull()
    await screen.findByText('merchant-2')
    act(() => useAuthStore.getState().setAuth({ accessToken: 'renewed', refreshToken: 'renewed', user: { id: 2, role: 'MERCHANT', merchant_id: 2 } }))
    expect(load).toHaveBeenCalledTimes(2)
  })

  it('discards a previous account request that completes after the switch', async () => {
    login(1)
    let finishOld!: (value: string) => void
    const old = new Promise<string>((resolve) => { finishOld = resolve })
    const load = vi.fn(() => useAuthStore.getState().user?.id === 1 ? old : Promise.resolve('new-account'))
    function Categories() {
      const { data } = useQuery({ queryKey: ['categories'], queryFn: load })
      return <div>{data}</div>
    }
    render(<SessionQueryProvider><Categories /></SessionQueryProvider>)
    await waitFor(() => expect(load).toHaveBeenCalledTimes(1))
    act(() => login(2))
    await screen.findByText('new-account')
    await act(async () => finishOld('old-account'))
    expect(screen.queryByText('old-account')).toBeNull()
    expect(screen.getByText('new-account')).toBeInTheDocument()
  })
})
