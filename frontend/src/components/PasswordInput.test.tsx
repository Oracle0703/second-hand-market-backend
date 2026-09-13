import { useState } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { expect, it } from 'vitest'
import { PasswordInput } from './PasswordInput'
import { passwordPattern } from '@/utils/password'
it('writes the generated password into the controlled form value', () => {
  function Form() {
    const [value, setValue] = useState('')
    return (
      <>
        <label htmlFor="initial-password">初始密码</label>
        <PasswordInput
          id="initial-password"
          value={value}
          onChange={setValue}
        />
      </>
    )
  }
  render(<Form />)
  fireEvent.click(screen.getByRole('button', { name: '生成随机密码' }))
  expect((screen.getByLabelText('初始密码') as HTMLInputElement).value).toMatch(
    passwordPattern
  )
})
