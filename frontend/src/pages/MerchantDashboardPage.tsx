import { useQuery } from '@tanstack/react-query'
import { http } from '../services/http'

export function MerchantDashboardPage() {
  const { data, isLoading, error } = useQuery({
    queryKey: ['merchant-dashboard'],
    queryFn: async () => (await http.get('/merchant/dashboard')).data.data as any
  })

  if (isLoading) return <p>加载中...</p>
  if (error) return <p className="error">{(error as Error).message}</p>

  return (
    <section className="card">
      <h1>商家仪表盘</h1>
      <pre>{JSON.stringify(data, null, 2)}</pre>
    </section>
  )
}
