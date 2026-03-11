export function generateDeviceID(): string {
  const rand = Math.random().toString(36).slice(2, 10)
  return `dev_${Date.now().toString(36)}_${rand}`
}
