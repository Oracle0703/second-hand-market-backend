import { useMemo } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { PageContainer, ProCard } from '@ant-design/pro-components'
import { Alert, Button, Space, Statistic, Table } from 'antd'
import { http, type APIResponse } from '@/services/http'
import { ExploreFilters } from './ExploreFilters'
import { groupLabels, platforms, readExploreLocation, typeLabels } from './explore'
import type { EventRow, ExploreReport, GroupRow, Scope } from './explore'
import './explore.css'

const beijingTime = new Intl.DateTimeFormat('sv-SE', { timeZone: 'Asia/Shanghai', dateStyle: 'short', timeStyle: 'medium' })
const baseEventColumns = [
  { title: '发生时间（北京）', dataIndex: 'occurred_at', render: (time: string) => beijingTime.format(new Date(time)) },
  { title: '事件名', dataIndex: 'name' },
  { title: '平台', dataIndex: 'platform', render: (value: string) => platforms[value] ?? value },
  { title: '页面', dataIndex: 'page' },
  { title: '来源', dataIndex: 'source', render: (value: string | null) => value || '—' },
  { title: '商品 ID', dataIndex: 'product_id', render: (value: number | null) => value ?? '—' }
]
function eventDetails(event: EventRow) {
  const entries = Object.entries(event.properties ?? {})
  return (
    <div className="explore-event-details">
      <div><strong>事件 ID</strong> <code>{event.event_id}</code></div>
      {entries.length ? <dl>{entries.map(([key, value]) => (
        <div key={key}><dt>{key}</dt><dd><code>{JSON.stringify(value)}</code></dd></div>
      ))}</dl> : <span className="muted">本条事件未上报自定义属性</span>}
    </div>
  )
}

export default function ExplorePage({ scope }: { scope: Scope }) {
  const [search, setSearch] = useSearchParams()
  const location = useMemo(() => readExploreLocation(search, scope), [search, scope])
  const { form, params, pageNo, pageSize } = location
  const report = useQuery({
    queryKey: [scope, 'analytics-explore', params, pageNo, pageSize],
    enabled: !location.error,
    queryFn: async ({ signal }) => (await http.get<APIResponse<ExploreReport>>('/' + scope + '/analytics/explore', {
      params: { ...params, page_no: pageNo, page_size: pageSize }, signal
    })).data.data
  })

  function apply(next: Record<string, string>, page: number, size: number) {
    const updated = new URLSearchParams({ ...next, page_no: String(page), page_size: String(size) })
    if (updated.toString() === search.toString()) void report.refetch()
    else setSearch(updated)
  }
  const groupName = form.group === 'property' ? form.groupKey : groupLabels[form.group]
  const groups = report.data?.groups ?? []
  const groupColumns = [
    { title: groupName, dataIndex: 'value', render: (value: string | null, row: GroupRow) => {
      if (value === null) return <span className="muted">未上报</span>
      if (form.group === 'platform') return platforms[value] ?? value
      if (form.group === 'property' && row.value_type) return <span>{value || '""'} <small className="muted">{typeLabels[row.value_type]}</small></span>
      return value || '（空值）'
    } },
    { title: '事件数', dataIndex: 'count', align: 'right' as const },
    { title: '去重设备数', dataIndex: 'devices', align: 'right' as const }
  ]
  const eventColumns = scope === 'admin' ? [...baseEventColumns, { title: '商户 ID', dataIndex: 'merchant_id' }] : baseEventColumns
  const draftKey = JSON.stringify([scope, form])
  return (
    <PageContainer title="事件查询">
      <Space direction="vertical" size="middle" style={{ display: 'flex' }}>
        <ProCard title="筛选条件" bordered>
          <ExploreFilters key={draftKey} initial={form} scope={scope} onApply={next => apply(next, 1, location.error ? 20 : pageSize)} />
        </ProCard>
        {location.error && <Alert type="error" showIcon message={location.error} />}
        {report.isLoading && <p role="status">正在查询事件…</p>}
        {report.isError && <Alert type="error" showIcon message="事件查询失败" description={report.error.message}
          action={<Button onClick={() => void report.refetch()}>重试</Button>} />}
        {!location.error && report.data && <>
          <ProCard bordered>
            <div className="explore-summary"><Statistic title="事件总数" value={report.data.total} /><Statistic title="去重设备数" value={report.data.devices} /></div>
            <p className="muted explore-note">按北京时间的事件发生日期统计。同一商户、同一平台内，同一安装设备在所选范围内计一次。</p>
          </ProCard>
          <ProCard title="分组统计" bordered>
            <p className="explore-note">按{groupName}分组，共 {report.data.groups_total} 组{report.data.groups_total > groups.length ? '，按事件数显示前 ' + groups.length + ' 组' : ''}。</p>
            <Table<GroupRow> columns={groupColumns} dataSource={groups} rowKey={row => JSON.stringify([row.value_type, row.value])}
              size="small" pagination={{ pageSize: 10, hideOnSinglePage: true, showSizeChanger: false }}
              locale={{ emptyText: '没有符合条件的分组' }} />
          </ProCard>
          <ProCard title="事件明细" bordered>
            <Table<EventRow> columns={eventColumns} dataSource={report.data.events} size="small" scroll={{ x: 850 }}
              rowKey={event => JSON.stringify([event.merchant_id, event.platform, event.event_id])}
              expandable={{ expandedRowRender: eventDetails, columnTitle: '属性', columnWidth: 56 }}
              pagination={{ current: pageNo, pageSize, total: report.data.total, showSizeChanger: true,
                pageSizeOptions: [10, 20, 50, 100], onChange: (page, size) => apply(params, size === pageSize ? page : 1, size),
                showTotal: total => '共 ' + total + ' 条' }}
              locale={{ emptyText: '没有符合条件的事件' }} />
          </ProCard>
        </>}
      </Space>
    </PageContainer>
  )
}
