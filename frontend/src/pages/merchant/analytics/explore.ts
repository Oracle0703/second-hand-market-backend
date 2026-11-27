import { lastSevenDays, rangeError } from './analyticsDates'

export type Scope = 'merchant' | 'admin'
export type ValueType = 'string' | 'number' | 'boolean'
type Operator = 'eq' | 'ne' | 'contains' | 'gte' | 'lte'
type Scalar = string | number | boolean

export type Condition = {
  id: number
  key: string
  type: ValueType
  op: Operator
  value: string
}
export type ExploreForm = {
  start: string
  end: string
  platform: string
  name: string
  page: string
  source: string
  product: string
  merchant: string
  group: string
  groupKey: string
  conditions: Condition[]
}
export type Dimensions = { names: string[]; pages: string[]; sources: string[]; property_keys: string[] }
export type EventRow = {
  event_id: string
  name: string
  page: string
  source: string | null
  product_id: number | null
  platform: string
  occurred_at: string
  merchant_id: number
  properties: Record<string, Scalar> | null
}
export type GroupRow = { value: string | null; value_type: ValueType | null; count: number; devices: number }
export type ExploreReport = {
  total: number
  devices: number
  groups: GroupRow[]
  groups_total: number
  events: EventRow[]
  page_no: number
  page_size: number
}

export const operators: Record<ValueType, Operator[]> = {
  string: ['eq', 'ne', 'contains'],
  number: ['eq', 'ne', 'gte', 'lte'],
  boolean: ['eq', 'ne']
}
export const operatorLabels: Record<Operator, string> = {
  eq: '等于', ne: '不等于', contains: '包含', gte: '大于或等于', lte: '小于或等于'
}
export const typeLabels: Record<ValueType, string> = { string: '文本', number: '数值', boolean: '布尔' }
export const groupLabels: Record<string, string> = {
  name: '事件名', date: '日期', platform: '平台', page: '页面', source: '来源',
  product_id: '商品 ID', merchant_id: '商户 ID', property: '自定义属性'
}
export const platforms: Record<string, string> = { weapp: '微信', tt: '抖音' }

export function initialForm(): ExploreForm {
  return { ...lastSevenDays(), platform: '', name: '', page: '', source: '', product: '', merchant: '', group: 'name', groupKey: '', conditions: [] }
}

export function scopeParams(form: ExploreForm, scope: Scope): Record<string, string> {
  const params: Record<string, string> = { start_date: form.start, end_date: form.end }
  if (form.platform) params.platform = form.platform
  if (scope === 'admin' && form.merchant.trim()) params.merchant_id = form.merchant.trim()
  return params
}

export function exploreParams(form: ExploreForm, scope: Scope): Record<string, string> {
  const invalidRange = rangeError(form.start, form.end)
  if (invalidRange) throw new Error(invalidRange)
  const params = scopeParams(form, scope)
  if (params.merchant_id && !/^[1-9][0-9]*$/.test(params.merchant_id)) throw new Error('商户 ID 应为正整数')
  if (form.product.trim() && !/^[1-9][0-9]*$/.test(form.product.trim())) throw new Error('商品 ID 应为正整数')
  if (form.name.trim()) params.name = form.name.trim()
  if (form.page.trim()) params.page = form.page.trim()
  if (form.source.trim()) params.source = form.source.trim()
  if (form.product.trim()) params.product_id = form.product.trim()
  if (form.group === 'property' && !form.groupKey.trim()) throw new Error('请输入用于分组的属性名')
  params.group_by = form.group === 'property' ? 'prop.' + form.groupKey.trim() : form.group
  if (form.conditions.length) {
    const filters = form.conditions.map((condition, index) => {
      const key = condition.key.trim()
      if (!key) throw new Error('请输入第 ' + (index + 1) + ' 条条件的属性名')
      let value: Scalar = condition.value
      if (condition.type === 'number') {
        value = Number(condition.value)
        if (!condition.value.trim() || !Number.isFinite(value)) throw new Error('第 ' + (index + 1) + ' 条条件需要有效数值')
      } else if (condition.type === 'boolean') {
        value = condition.value === 'true'
      } else if (condition.op === 'contains' && !value) {
        throw new Error('请输入第 ' + (index + 1) + ' 条条件要包含的文本')
      }
      return { key, op: condition.op, value }
    })
    params.filters = JSON.stringify(filters)
  }
  return params
}

function conditionsFromURL(raw: string | null): Condition[] {
  if (!raw) return []
  const entries: unknown = JSON.parse(raw)
  if (!Array.isArray(entries) || entries.length > 8) throw new Error('链接中的属性条件无效')
  return entries.map((entry, index) => {
    if (!entry || typeof entry.key !== 'string') throw new Error('链接中的属性名无效')
    const type = typeof entry.value
    if (type !== 'string' && type !== 'number' && type !== 'boolean') throw new Error('链接中的属性值无效')
    if (!operators[type].includes(entry.op)) throw new Error('链接中的比较方式无效')
    return { id: index + 1, key: entry.key, type, op: entry.op, value: String(entry.value) }
  })
}

export function readExploreLocation(search: URLSearchParams, scope: Scope) {
  const form = initialForm()
  form.start = search.get('start_date') ?? form.start
  form.end = search.get('end_date') ?? form.end
  for (const key of ['platform', 'name', 'page', 'source'] as const) form[key] = search.get(key) ?? ''
  form.product = search.get('product_id') ?? ''
  form.merchant = scope === 'admin' ? search.get('merchant_id') ?? '' : ''
  const group = search.get('group_by') || 'name'
  form.group = group.startsWith('prop.') ? 'property' : group
  if (form.group === 'property') form.groupKey = group.slice(5)
  const pageNo = Number(search.get('page_no') || 1)
  const pageSize = Number(search.get('page_size') || 20)
  let error = ''
  let params: Record<string, string> = {}
  try {
    if (!Number.isSafeInteger(pageNo) || pageNo < 1 || !Number.isInteger(pageSize) || pageSize < 1 || pageSize > 100) throw new Error('链接中的分页参数无效')
    if (form.platform && !Object.keys(platforms).includes(form.platform)) throw new Error('链接中的平台无效')
    if (!Object.keys(groupLabels).includes(form.group) || (scope === 'merchant' && form.group === 'merchant_id')) throw new Error('链接中的分组维度不可用')
    form.conditions = conditionsFromURL(search.get('filters'))
    params = exploreParams(form, scope)
  } catch (err) {
    error = err instanceof SyntaxError ? '链接中的属性条件格式无效' : (err as Error).message
  }
  return { form, params, error, pageNo, pageSize }
}
