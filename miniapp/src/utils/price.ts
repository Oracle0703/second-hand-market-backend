export function centToYuanText(cent?: number | null) {
  const safeCent = Number(cent ?? 0)
  if (!Number.isFinite(safeCent)) return '0.00'
  return (safeCent / 100).toFixed(2)
}
