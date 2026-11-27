import { useRef, useState, type FormEvent, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Alert, Button, Input, Space } from 'antd'
import { http, type APIResponse } from '@/services/http'
import { exploreParams, groupLabels, initialForm, operatorLabels, operators, platforms, scopeParams, typeLabels } from './explore'
import type { Condition, Dimensions, ExploreForm, Scope, ValueType } from './explore'
import { rangeError } from './analyticsDates'

function Field({ label, children }: { label: string; children: ReactNode }) {
  return <label className="explore-field"><span>{label}</span>{children}</label>
}

function useDimensions(scope: Scope, params: Record<string, string>, enabled: boolean) {
  return useQuery({
    queryKey: [scope, 'analytics-dimensions', params],
    enabled,
    staleTime: 30_000,
    queryFn: async ({ signal }) => (await http.get<APIResponse<Dimensions>>('/' + scope + '/analytics/dimensions', { params, signal })).data.data
  })
}

export function ExploreFilters({ scope, initial, onApply }: { scope: Scope; initial: ExploreForm; onApply: (params: Record<string, string>) => void }) {
  const [form, setForm] = useState(initial)
  const [error, setError] = useState('')
  const nextId = useRef(initial.conditions.length + 1)
  const range = scopeParams(initial, scope)
  const validRange = !rangeError(initial.start, initial.end) && (!range.merchant_id || /^[1-9][0-9]*$/.test(range.merchant_id))
  const dimensions = useDimensions(scope, range, validRange)
  const eventName = initial.name.trim()
  const eventDimensions = useDimensions(scope, { ...range, name: eventName }, validRange && !!eventName)
  const fields = eventName ? eventDimensions : dimensions
  function setValue<K extends keyof ExploreForm>(key: K, value: ExploreForm[K]) {
    setForm(previous => ({ ...previous, [key]: value }))
    setError('')
  }
  function changeCondition(id: number, change: Partial<Condition>) {
    setValue('conditions', form.conditions.map(item => item.id === id ? { ...item, ...change } : item))
  }
  function apply(event: FormEvent) {
    event.preventDefault()
    try {
      const params = exploreParams(form, scope)
      setError('')
      onApply(params)
    } catch (err) {
      setError((err as Error).message)
    }
  }
  function reset() {
    const next = initialForm()
    setForm(next)
    setError('')
    onApply(exploreParams(next, scope))
  }
  const keys = fields.data?.property_keys ?? []
  return (
    <form className="explore-filter-form" onSubmit={apply}>
      <div className="explore-filter-grid">
        <Field label="开始日期"><Input type="date" value={form.start} onChange={e => setValue('start', e.target.value)} /></Field>
        <Field label="结束日期"><Input type="date" value={form.end} onChange={e => setValue('end', e.target.value)} /></Field>
        <Field label="平台"><select value={form.platform} onChange={e => setValue('platform', e.target.value)}>
          <option value="">全部平台</option>
          {Object.entries(platforms).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
        </select></Field>
        <Field label="商品 ID"><Input value={form.product} inputMode="numeric" placeholder="全部商品" onChange={e => setValue('product', e.target.value)} /></Field>
        {scope === 'admin' && <Field label="商户 ID"><Input value={form.merchant} inputMode="numeric" placeholder="全部商户" onChange={e => setValue('merchant', e.target.value)} /></Field>}
        {([
          ['name', '事件名', 'names'], ['page', '页面', 'pages'], ['source', '来源', 'sources']
        ] as const).map(([field, label, list]) => (
          <Field label={label} key={field}>
            <Input list={'explore-' + list} value={form[field]} placeholder={'选择或输入' + label} onChange={e => setValue(field, e.target.value)} />
            <datalist id={'explore-' + list}>{(field === 'name' ? dimensions : fields).data?.[list]?.map(value => <option key={value} value={value} />)}</datalist>
          </Field>
        ))}
        <Field label="分组维度"><select value={form.group} onChange={e => setValue('group', e.target.value)}>
          {Object.entries(groupLabels).filter(([key]) => scope === 'admin' || key !== 'merchant_id').map(([key, label]) => <option key={key} value={key}>{label}</option>)}
        </select></Field>
        {form.group === 'property' && <Field label="分组属性名"><Input list="explore-properties" value={form.groupKey} placeholder="选择或输入属性名" onChange={e => setValue('groupKey', e.target.value)} /></Field>}
      </div>
      <datalist id="explore-properties">{keys.map(key => <option key={key} value={key} />)}</datalist>
      {(dimensions.isError || fields.isError) && <Alert type="warning" message="候选字段加载失败，可直接输入字段名称" showIcon />}
      <div className="explore-condition-header">
        <strong>自定义属性条件{form.conditions.length > 1 && '（同时满足）'}</strong>
        <Button size="small" disabled={form.conditions.length >= 8}
          onClick={() => setValue('conditions', [...form.conditions, { id: nextId.current++, key: '', type: 'string', op: 'eq', value: '' }])}>添加条件</Button>
      </div>
      {form.conditions.map((condition, index) => {
        const n = index + 1
        return (
          <div className="explore-condition" key={condition.id}>
            <Field label={'属性名 ' + n}><Input list="explore-properties" value={condition.key} placeholder="选择或输入属性名" onChange={e => changeCondition(condition.id, { key: e.target.value })} /></Field>
            <Field label={'类型 ' + n}><select value={condition.type} onChange={e => {
              const type = e.target.value as ValueType
              changeCondition(condition.id, { type, value: type === 'boolean' ? 'true' : '', op: 'eq' })
            }}>
              {Object.entries(typeLabels).map(([type, label]) => <option key={type} value={type}>{label}</option>)}
            </select></Field>
            <Field label={'比较方式 ' + n}><select value={condition.op} onChange={e => changeCondition(condition.id, { op: e.target.value as Condition['op'] })}>
              {operators[condition.type].map(op => <option key={op} value={op}>{operatorLabels[op]}</option>)}
            </select></Field>
            <Field label={'值 ' + n}>
              {condition.type === 'boolean' ? <select value={condition.value} onChange={e => changeCondition(condition.id, { value: e.target.value })}>
                <option value="true">true</option><option value="false">false</option>
              </select> : <Input type={condition.type === 'number' ? 'number' : 'text'} step="any" value={condition.value} onChange={e => changeCondition(condition.id, { value: e.target.value })} />}
            </Field>
            <Button aria-label={'移除条件 ' + n} onClick={() => setValue('conditions', form.conditions.filter(item => item.id !== condition.id))}>移除</Button>
          </div>
        )
      })}
      {error && <Alert type="error" showIcon message={error} />}
      <Space><Button htmlType="submit" type="primary">查询</Button><Button onClick={reset}>重置筛选</Button></Space>
    </form>
  )
}
