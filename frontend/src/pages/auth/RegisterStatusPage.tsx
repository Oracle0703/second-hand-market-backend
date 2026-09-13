import { Alert, Button } from 'antd'
import { useAuthStore } from '@/stores/auth-store'
import { useNavigate } from 'react-router-dom'
export function RegisterStatusPage() {
  const clear = useAuthStore((state) => state.clear)
  const navigate = useNavigate()
  return (
    <div style={{ padding: 32 }}>
      <Alert type="info" message="账号尚未开通，请联系管理员处理。" />
      <Button
        onClick={() => {
          clear()
          navigate('/login')
        }}
      >
        返回登录
      </Button>
    </div>
  )
}
