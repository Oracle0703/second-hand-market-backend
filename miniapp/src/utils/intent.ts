export function toBuyerStatusText(status: string): string {
  if (status === 'CONTACTED') return '已联系'
  if (status === 'CLOSED') return '已关闭'
  return '处理中'
}
