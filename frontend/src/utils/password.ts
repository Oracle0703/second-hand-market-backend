export const passwordPattern =
  /^(?=.*[A-Z])(?=.*[a-z])(?=.*[0-9])(?=.*[^A-Za-z0-9])[!-~]{12,72}$/
export const passwordHelp =
  '12–72 位，包含大写字母、小写字母、数字和符号，不含空格'

function randomIndex(size: number): number {
  const bytes = new Uint32Array(1)
  const bound = Math.floor(0x100000000 / size) * size
  do {
    crypto.getRandomValues(bytes)
  } while (bytes[0] >= bound)
  return bytes[0] % size
}

export function generatePassword(): string {
  const groups = [
    'ABCDEFGHJKLMNPQRSTUVWXYZ',
    'abcdefghijkmnopqrstuvwxyz',
    '23456789',
    '!@#$%&*+-=?'
  ]
  const alphabet = groups.join('')
  const result = groups.map((group) => group[randomIndex(group.length)])
  while (result.length < 20) result.push(alphabet[randomIndex(alphabet.length)])
  for (let i = result.length - 1; i > 0; i--) {
    const j = randomIndex(i + 1)
    ;[result[i], result[j]] = [result[j], result[i]]
  }
  return result.join('')
}
