export function nextFavoriteState(current: boolean, action: 'add' | 'remove'): boolean {
  if (action === 'add') return true
  return false
}
