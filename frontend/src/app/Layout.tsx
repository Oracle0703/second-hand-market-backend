import { useState } from 'react'
import {
  AppstoreOutlined,
  AuditOutlined,
  FileTextOutlined,
  HomeOutlined,
  LogoutOutlined,
  OrderedListOutlined,
  SafetyCertificateOutlined,
  SettingOutlined,
  ShoppingOutlined
} from '@ant-design/icons'
import { ProLayout, type MenuDataItem } from '@ant-design/pro-components'
import { Button } from 'antd'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { api } from '../services/api'
import { useAuthStore } from '../stores/auth-store'

const adminMenus: MenuDataItem[] = [
  { path: '/admin/merchants/reviews', name: '商家审核', icon: <AuditOutlined /> },
  { path: '/admin/logs', name: '全局日志', icon: <FileTextOutlined /> },
  { path: '/admin/account/security', name: '安全设置', icon: <SafetyCertificateOutlined /> }
]

const merchantMenus: MenuDataItem[] = [
  { path: '/merchant/dashboard', name: '全局', icon: <HomeOutlined /> },
  { path: '/merchant/products', name: '商品', icon: <ShoppingOutlined /> },
  { path: '/merchant/orders', name: '订单', icon: <OrderedListOutlined /> },
  { path: '/merchant/intents', name: '意向', icon: <AppstoreOutlined /> },
  { path: '/merchant/account', name: '账户', icon: <SettingOutlined /> },
  { path: '/merchant/logs', name: '操作日志', icon: <FileTextOutlined /> }
]

export function Layout() {
  const navigate = useNavigate()
  const location = useLocation()
  const { clear, user } = useAuthStore()
  const isAdmin = String(user?.role || '').includes('ADMIN')
  const menuData = isAdmin ? adminMenus : merchantMenus
  const [loggingOut, setLoggingOut] = useState(false)

  const logout = async () => {
    if (loggingOut) return
    const destination = isAdmin ? '/admin/login' : '/login'
    setLoggingOut(true)
    try {
      await api.logout()
    } catch {
      // Local logout must proceed even when server revocation fails.
    } finally {
      clear()
      navigate(destination)
    }
  }

  return (
    <ProLayout
      title="广汉市瑞扬家具经营部"
      logo={false}
      layout="side"
      fixedHeader
      fixSiderbar
      siderWidth={220}
      location={{ pathname: location.pathname }}
      route={{ routes: menuData }}
      menuItemRender={(item, dom) => (
        <a
          onClick={(e) => {
            e.preventDefault()
            if (item.path) navigate(item.path)
          }}
          href={item.path}
        >
          {dom}
        </a>
      )}
      actionsRender={() => [
        <span key="role" className="topbar-role">
          {String(user?.role || '')}
        </span>,
        <Button
          key="logout"
          type="link"
          icon={<LogoutOutlined />}
          loading={loggingOut}
          disabled={loggingOut}
          onClick={logout}
        >
          退出
        </Button>
      ]}
      onMenuHeaderClick={() => navigate(isAdmin ? '/admin/merchants/reviews' : '/merchant/dashboard')}
      contentStyle={{ padding: 16 }}
    >
      <Outlet />
    </ProLayout>
  )
}
