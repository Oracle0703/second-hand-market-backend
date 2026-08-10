const PRODUCT_STATUS_TEXT: Record<string, string> = {
  DRAFT: '草稿',
  ON_SHELF: '在售',
  LOCKED: '锁定',
  OFF_SHELF: '下架',
  SOLD: '售罄'
}

export function getProductStatusText(status: string): string {
  return PRODUCT_STATUS_TEXT[status] ?? status
}

export function canContactForProduct(status: string, canSubmitIntent?: boolean): boolean {
  return status === 'ON_SHELF' && canSubmitIntent !== false
}
