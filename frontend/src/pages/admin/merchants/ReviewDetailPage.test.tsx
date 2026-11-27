import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { expect, it, vi } from 'vitest'
import { ReviewDetailPage } from './ReviewDetailPage'

vi.mock('@/services/api', () => ({
  api: {
    adminMerchantReviewDetail: async () => ({
      data: { data: { merchant_detail: { merchant_name: '测试商户' }, account: null, audit_logs: [] } }
    })
  }
}))

it('shows the missing owner without offering account actions', async () => {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter initialEntries={['/admin/merchants/reviews/42']}>
        <Routes>
          <Route path="/admin/merchants/reviews/:merchantId" element={<ReviewDetailPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>
  )

  expect(await screen.findByText('该商户缺少店主账号')).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: '重置密码' })).not.toBeInTheDocument()
})
