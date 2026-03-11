import { Link, Outlet, useNavigate } from 'react-router-dom'
import { useAuthStore } from '../stores/auth-store'

export function Layout() {
  const navigate = useNavigate()
  const { clear, user } = useAuthStore()

  const logout = () => {
    clear()
    navigate('/login')
  }

  return (
    <div className="layout">
      <header>
        <strong>Second-hand Merchant Console</strong>
        <nav>
          <Link to="/merchant/dashboard">Dashboard</Link>
          <Link to="/merchant/products">商品</Link>
          <Link to="/merchant/orders">订单</Link>
          <Link to="/merchant/intents">意向</Link>
          <Link to="/admin/merchants/reviews">审核</Link>
        </nav>
        <div>
          <span>{user?.role}</span>
          <button onClick={logout}>退出</button>
        </div>
      </header>
      <main>
        <Outlet />
      </main>
    </div>
  )
}
