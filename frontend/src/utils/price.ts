export function centToYuanNumber(cent?: number | null) {
  const safe = Number(cent ?? 0)
  return Number.isFinite(safe) ? Number((safe / 100).toFixed(2)) : 0
}

export function centToYuanText(cent?: number | null) {
  return centToYuanNumber(cent).toFixed(2)
}

export function yuanToCent(yuan?: number | null) {
  const safe = Number(yuan ?? 0)
  if (!Number.isFinite(safe)) return 0
  return Math.round(safe * 100)
}
