const day = 24 * 60 * 60 * 1000
const beijingDate = new Intl.DateTimeFormat('sv-SE', { timeZone: 'Asia/Shanghai' })

export function lastSevenDays() {
  const now = Date.now()
  return { start: beijingDate.format(now - 6 * day), end: beijingDate.format(now) }
}

export function rangeError(start: string, end: string) {
  if (!start || !end) return '请选择开始日期和结束日期'
  if (start > end) return '开始日期应早于或等于结束日期'
  if (Date.parse(end) - Date.parse(start) >= 93 * day) return '单次最多查询 93 天'
}
