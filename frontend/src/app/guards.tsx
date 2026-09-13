import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { useAuthStore } from '../stores/auth-store'

export function RequireAuth({ role, scope }: { role?: 'ADMIN' | 'MERCHANT'; scope?: 'full' | 'onboarding' }) {
  const location = useLocation()
  const { accessToken, user, tokenScope } = useAuthStore()
  if (!accessToken || !user) {
    return <Navigate to="/login" replace />
  }
  if (role === 'ADMIN' && !String(user.role).includes('ADMIN')) {
    return <Navigate to="/merchant/dashboard" replace />
  }
  if (role === 'MERCHANT' && String(user.role).includes('ADMIN')) {
    return <Navigate to="/admin/merchants/reviews" replace />
  }
  if (scope && tokenScope !== scope && scope === 'full') {
    return <Navigate to="/register/status" replace />
  }
  if (user.must_change_password && location.pathname !== '/merchant/account') {
    return <Navigate to="/merchant/account" replace />
  }
  return <Outlet />
}
