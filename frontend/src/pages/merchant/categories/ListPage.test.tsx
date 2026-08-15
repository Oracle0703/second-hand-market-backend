import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '@/services/api'
import { ListPage } from './ListPage'

vi.mock('@/services/api', () => ({
  api: {
    categories: vi.fn(),
    createCategory: vi.fn(),
    updateCategory: vi.fn(),
    deleteCategory: vi.fn()
  }
}))

vi.mock('@ant-design/pro-components', async () => {
  const React = await vi.importActual<typeof import('react')>('react')
  return {
    PageContainer: ({ title, extra, children }: { title: string; extra?: ReactNode; children?: ReactNode }) =>
      React.createElement('main', null, React.createElement('h1', null, title), extra, children)
  }
})

function renderWithQuery(ui: ReactNode) {
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      {ui}
    </QueryClientProvider>
  )
}

describe('Category ListPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(api.categories).mockImplementation(async (level?: 1 | 2) => {
      if (level === 1) {
        return {
          data: {
            data: {
              items: [{ id: 1, merchant_id: 10, level: 1, name: '家具类', status: 'ENABLED', sort: 1 }]
            }
          }
        } as never
      }
      return {
        data: {
          data: {
            items: [{ id: 2, merchant_id: 10, parent_id: 1, level: 2, name: '沙发', status: 'DISABLED', sort: 1 }]
          }
        }
      } as never
    })
    vi.mocked(api.createCategory).mockResolvedValue({ data: { code: 0 } } as never)
    vi.mocked(api.updateCategory).mockResolvedValue({ data: { code: 0 } } as never)
    vi.mocked(api.deleteCategory).mockResolvedValue({ data: { code: 0 } } as never)
  })

  it('renders merchant categories and supports create, edit, and delete actions', async () => {
    renderWithQuery(<ListPage />)

    expect(await screen.findByText('家具类')).toBeInTheDocument()
    expect(screen.getByText('沙发')).toBeInTheDocument()
    expect(screen.getByText('停用')).toBeInTheDocument()
    expect(api.categories).toHaveBeenCalledWith(1, undefined, 'ALL')
    expect(api.categories).toHaveBeenCalledWith(2, undefined, 'ALL')

    fireEvent.click(screen.getByRole('button', { name: '新增一级分类' }))
    fireEvent.change(screen.getByLabelText('分类名称'), { target: { value: '家电类' } })
    fireEvent.click(screen.getByRole('button', { name: '保存' }))
    await waitFor(() => {
      expect(api.createCategory).toHaveBeenCalledWith({ level: 1, name: '家电类', sort: 0 })
    })

    fireEvent.click(screen.getByRole('button', { name: '编辑 沙发' }))
    fireEvent.change(screen.getByLabelText('分类名称'), { target: { value: '真皮沙发' } })
    fireEvent.click(screen.getByRole('button', { name: '保存' }))
    await waitFor(() => {
      expect(api.updateCategory).toHaveBeenCalledWith(2, { name: '真皮沙发', sort: 1, status: 'DISABLED' })
    })

    fireEvent.click(screen.getByRole('button', { name: '删除 沙发' }))
    await waitFor(() => {
      expect(api.deleteCategory).toHaveBeenCalledWith(2)
    })
  }, 15000)
})
