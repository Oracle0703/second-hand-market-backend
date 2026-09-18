// Merchant category API contract. Pages consume this shape only; compatibility
// with historical PascalCase responses lives in the HTTP service boundary.
export type Category = {
  id: number
  merchant_id?: number
  parent_id?: number | null
  level: 1 | 2
  name: string
  status: string
  sort: number
}

export type CategoryWire = Partial<Category> & {
  ID?: number
  MerchantID?: number
  ParentID?: number | null
  Level?: number
  Name?: string
  Status?: string
  Sort?: number
}

export function normalizeCategory(item: CategoryWire): Category {
  return {
    id: Number(item.id ?? item.ID ?? 0),
    merchant_id: item.merchant_id ?? item.MerchantID,
    parent_id: item.parent_id !== undefined ? item.parent_id : item.ParentID,
    level: Number(item.level ?? item.Level ?? 1) as 1 | 2,
    name: item.name ?? item.Name ?? '',
    status: item.status ?? item.Status ?? 'ENABLED',
    sort: Number(item.sort ?? item.Sort ?? 0)
  }
}
