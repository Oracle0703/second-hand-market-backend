import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { RegisterPage } from './RegisterPage'

const mockNavigate = vi.fn()
const mockPresign = vi.fn()
const mockUploadFile = vi.fn()
const mockRegister = vi.fn()

vi.mock('@ant-design/pro-components', () => import('@/test/pro-components-stub'))

vi.mock('@/services/api', () => ({
  api: {
    presign: (...args: unknown[]) => mockPresign(...args),
    uploadFile: (...args: unknown[]) => mockUploadFile(...args),
    register: (...args: unknown[]) => mockRegister(...args)
  }
}))

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom')
  return {
    ...actual,
    useNavigate: () => mockNavigate
  }
})

function renderPage() {
  return render(
    <MemoryRouter>
      <RegisterPage />
    </MemoryRouter>
  )
}

function fillRegistrationForm() {
  fireEvent.change(screen.getByLabelText('商家名称'), { target: { value: 'Claim Store' } })
  fireEvent.change(screen.getByLabelText('联系人姓名'), { target: { value: 'Owner' } })
  fireEvent.change(screen.getByLabelText('联系电话'), { target: { value: '13800138000' } })
  fireEvent.change(screen.getByLabelText('登录账号'), { target: { value: 'claim_owner' } })
  fireEvent.change(screen.getByLabelText('登录密码'), { target: { value: 'Passw0rd!2026' } })
}

describe('RegisterPage license capability', () => {
  beforeEach(() => {
    mockNavigate.mockReset()
    mockPresign.mockReset()
    mockUploadFile.mockReset()
    mockRegister.mockReset()
    localStorage.clear()
  })

  it('submits the one-time license token without persisting it', async () => {
    mockPresign.mockResolvedValue({
      data: { data: { file_id: 42, object_key: 'merchant_license/f.jpg', file_token: 'raw-capability' } }
    })
    mockUploadFile.mockResolvedValue({ data: { data: { file_id: 42, status: 'PASS' } } })
    mockRegister.mockResolvedValue({ data: { data: { merchant_id: 9 } } })

    const { container } = renderPage()
    const input = container.querySelector('input[type="file"]') as HTMLInputElement
    fireEvent.change(input, {
      target: { files: [new File(['jpeg'], 'license.jpg', { type: 'image/jpeg' })] }
    })
    await screen.findByText(/file_id: 42/)
    fillRegistrationForm()
    fireEvent.click(screen.getByRole('button', { name: '提交注册' }))

    await waitFor(() => {
      expect(mockRegister).toHaveBeenCalledWith(
        expect.objectContaining({
          license_file_id: 42,
          license_file_token: 'raw-capability'
        })
      )
    })
    const uploadForm = mockUploadFile.mock.calls[0][0] as FormData
    expect(uploadForm.get('file_token')).toBe('raw-capability')
    expect(localStorage.getItem('file_token')).toBeNull()
  })

  it('keeps the previous complete license when replacement upload fails', async () => {
    mockPresign
      .mockResolvedValueOnce({ data: { data: { file_id: 42, object_key: 'merchant_license/first.jpg', file_token: 'first-token' } } })
      .mockResolvedValueOnce({ data: { data: { file_id: 43, object_key: 'merchant_license/second.jpg', file_token: 'second-token' } } })
    mockUploadFile.mockResolvedValueOnce({ data: { data: { file_id: 42, status: 'PASS' } } }).mockRejectedValueOnce(new Error('replacement failed'))
    mockRegister.mockResolvedValue({ data: { data: { merchant_id: 9 } } })

    const { container } = renderPage()
    const input = container.querySelector('input[type="file"]') as HTMLInputElement
    fireEvent.change(input, {
      target: { files: [new File(['first'], 'first.jpg', { type: 'image/jpeg' })] }
    })
    await screen.findByText(/file_id: 42/)
    fireEvent.change(input, {
      target: { files: [new File(['second'], 'second.jpg', { type: 'image/jpeg' })] }
    })
    await screen.findByText('replacement failed')

    expect(screen.getByText(/已上传：first.jpg.*file_id: 42/)).toBeTruthy()
    fillRegistrationForm()
    fireEvent.click(screen.getByRole('button', { name: '提交注册' }))

    await waitFor(() => {
      expect(mockRegister).toHaveBeenCalledWith(
        expect.objectContaining({
          license_file_id: 42,
          license_file_token: 'first-token'
        })
      )
    })
  })
})
