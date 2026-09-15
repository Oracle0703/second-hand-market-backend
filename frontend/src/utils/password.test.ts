import { describe, expect, it } from 'vitest'
import { generatePassword, passwordPattern } from './password'

describe('secure initial passwords', () => {
  it('always supplies 20 characters with English letters and digits', () => {
    const passwords = Array.from({ length: 200 }, generatePassword)
    expect(new Set(passwords).size).toBe(200)
    for (const password of passwords) {
      expect(password).toHaveLength(20)
      expect(password).toMatch(passwordPattern)
    }
  })
  it('rejects passwords without English letters or digits, whitespace and bcrypt-overflow passwords', () => {
    for (const password of [
      '12345678',
      '123456789012',
      'abcdefghijkl',
      'Abcdef12! ',
      'A1!' + 'x'.repeat(70)
    ])
      expect(passwordPattern.test(password)).toBe(false)
  })
})
