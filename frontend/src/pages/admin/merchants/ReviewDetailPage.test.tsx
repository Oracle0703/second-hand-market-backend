import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ReviewDetailPage } from './ReviewDetailPage'

const mockAdminMerchantReviewDetail = vi.fn()
const mockAdminLicenseContent = vi.fn()
const mockCreateObjectURL = vi.fn()
const mockRevokeObjectURL = vi.fn()

vi.mock('@ant-design/pro-components', () => ({
  PageContainer: ({ children }: { children?: React.ReactNode }) => <main>{children}</main>,
  ProCard: ({ title, children }: { title?: React.ReactNode; children?: React.ReactNode }) => (
    <section>
      {title ? <h2>{title}</h2> : null}
      {children}
    </section>
  ),
  ProDescriptions: () => <section aria-label="商家资料" />,
  ProTable: () => <section aria-label="审核日志" />
}))

vi.mock('@/services/api', () => ({
  api: {
    adminMerchantReviewDetail: (...args: unknown[]) => mockAdminMerchantReviewDetail(...args),
    adminLicenseContent: (...args: unknown[]) => mockAdminLicenseContent(...args),
    adminMerchantApprove: vi.fn(),
    adminMerchantReject: vi.fn()
  }
}))

function merchantDetail(licenseFileID: number | null) {
  return {
    merchant_detail: {
      id: 9,
      merchant_no: 'M0009',
      merchant_name: 'Private License Store',
      contact_name: 'Owner',
      contact_phone: '13800138000',
      license_file_id: licenseFileID,
      review_status: 'PENDING',
      reject_reason: null,
      reviewed_by: null,
      reviewed_at: null,
      created_at: '2026-07-26T00:00:00Z',
      updated_at: '2026-07-26T00:00:00Z'
    },
    audit_logs: []
  }
}

function resolvedDetail(licenseFileID: number | null) {
  return { data: { data: merchantDetail(licenseFileID) } }
}

function renderPage() {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false }
    }
  })
  const view = render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={['/admin/merchants/9']}>
        <Routes>
          <Route path="/admin/merchants/:merchantId" element={<ReviewDetailPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>
  )
  return { ...view, client }
}

function axiosStatusError(status: number) {
  return Object.assign(new Error(`request failed with ${status}`), {
    isAxiosError: true,
    response: { status }
  })
}

function storageContents(storage: Storage) {
  return [...Array(storage.length)].flatMap((_, index) => {
    const key = storage.key(index)
    return [key, key === null ? null : storage.getItem(key)]
  }).join(' ')
}

describe('ReviewDetailPage private license preview', () => {
  beforeEach(() => {
    mockAdminMerchantReviewDetail.mockReset()
    mockAdminLicenseContent.mockReset()
    mockCreateObjectURL.mockReset()
    mockRevokeObjectURL.mockReset()
    localStorage.clear()
    sessionStorage.clear()
    Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: mockCreateObjectURL })
    Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: mockRevokeObjectURL })
  })

  it('shows an empty state without requesting content when no license is bound', async () => {
    mockAdminMerchantReviewDetail.mockResolvedValue(resolvedDetail(null))
    renderPage()

    expect(await screen.findByText('暂无营业执照')).toBeTruthy()
    expect(mockAdminLicenseContent).not.toHaveBeenCalled()
  })

  it('keeps a stable preview region while the private blob is loading', async () => {
    mockAdminMerchantReviewDetail.mockResolvedValue(resolvedDetail(42))
    mockAdminLicenseContent.mockImplementation(() => new Promise(() => undefined))
    renderPage()

    const loading = await screen.findByLabelText('营业执照加载中')
    const region = loading.closest('[data-license-preview]') as HTMLElement
    expect(region).toBeTruthy()
    expect(region.style.minHeight).toBe('280px')
  })

  it('renders the authenticated blob and keeps it out of browser storage', async () => {
    const blob = new Blob(['license'], { type: 'image/jpeg' })
    mockAdminMerchantReviewDetail.mockResolvedValue(resolvedDetail(42))
    mockAdminLicenseContent.mockResolvedValue({ data: blob })
    mockCreateObjectURL.mockReturnValue('blob:private-license-42')
    renderPage()

    const image = await screen.findByRole('img', { name: '营业执照' })
    expect(image.getAttribute('src')).toBe('blob:private-license-42')
    expect(screen.getByText('文件 ID：42')).toBeTruthy()
    expect(storageContents(localStorage)).not.toContain('blob:')
    expect(storageContents(sessionStorage)).not.toContain('blob:')
  })

  it.each([
    { status: 403, message: '营业执照鉴权失败' },
    { status: 404, message: '营业执照不可用' }
  ])('shows the expected state for HTTP $status', async ({ status, message }) => {
    mockAdminMerchantReviewDetail.mockResolvedValue(resolvedDetail(42))
    mockAdminLicenseContent.mockRejectedValue(axiosStatusError(status))
    renderPage()

    expect(await screen.findByText(message)).toBeTruthy()
    expect(screen.queryByRole('img', { name: '营业执照' })).toBeNull()
  })

  it('shows unavailable when the browser cannot decode the blob image', async () => {
    mockAdminMerchantReviewDetail.mockResolvedValue(resolvedDetail(42))
    mockAdminLicenseContent.mockResolvedValue({ data: new Blob(['not-an-image'], { type: 'image/jpeg' }) })
    mockCreateObjectURL.mockReturnValue('blob:broken-license')
    renderPage()

    const image = await screen.findByRole('img', { name: '营业执照' })
    fireEvent.error(image)
    expect(await screen.findByText('营业执照不可用')).toBeTruthy()
  })

  it('revokes object URLs when the file changes and when the page unmounts', async () => {
    mockAdminMerchantReviewDetail.mockResolvedValue(resolvedDetail(42))
    mockAdminLicenseContent.mockImplementation(async (fileID: number) => ({
      data: new Blob([`license-${fileID}`], { type: 'image/jpeg' })
    }))
    mockCreateObjectURL.mockReturnValueOnce('blob:license-42').mockReturnValueOnce('blob:license-43')
    const { client, unmount } = renderPage()

    expect((await screen.findByRole('img', { name: '营业执照' })).getAttribute('src')).toBe('blob:license-42')
    act(() => {
      client.setQueryData(['admin-merchant-detail', '9'], merchantDetail(43))
    })
    await waitFor(() => {
      expect(screen.getByRole('img', { name: '营业执照' }).getAttribute('src')).toBe('blob:license-43')
    })
    expect(mockRevokeObjectURL).toHaveBeenCalledWith('blob:license-42')

    unmount()
    expect(mockRevokeObjectURL).toHaveBeenCalledWith('blob:license-43')
  })
})
