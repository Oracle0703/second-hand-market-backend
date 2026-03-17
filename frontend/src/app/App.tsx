import { Suspense, lazy, type ReactNode } from 'react'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { RequireAuth } from './guards'

const Layout = lazy(() => import('./Layout').then((m) => ({ default: m.Layout })))
const LoginPage = lazy(() => import('@/pages/auth/LoginPage').then((m) => ({ default: m.LoginPage })))
const AdminLoginPage = lazy(() => import('@/pages/auth/AdminLoginPage').then((m) => ({ default: m.AdminLoginPage })))
const RegisterPage = lazy(() => import('@/pages/auth/RegisterPage').then((m) => ({ default: m.RegisterPage })))
const RegisterStatusPage = lazy(() => import('@/pages/auth/RegisterStatusPage').then((m) => ({ default: m.RegisterStatusPage })))
const AdminReviewsPage = lazy(() => import('@/pages/admin/merchants/ReviewsPage').then((m) => ({ default: m.ReviewsPage })))
const DashboardPage = lazy(() => import('@/pages/merchant/dashboard/DashboardPage').then((m) => ({ default: m.DashboardPage })))
const ProductListPage = lazy(() => import('@/pages/merchant/products/ListPage').then((m) => ({ default: m.ListPage })))
const OrderListPage = lazy(() => import('@/pages/merchant/orders/ListPage').then((m) => ({ default: m.ListPage })))
const ProductCreatePage = lazy(() => import('@/pages/merchant/products/CreatePage').then((m) => ({ default: m.CreatePage })))
const ProductEditPage = lazy(() => import('@/pages/merchant/products/EditPage').then((m) => ({ default: m.EditPage })))
const ProductDetailPage = lazy(() => import('@/pages/merchant/products/DetailPage').then((m) => ({ default: m.DetailPage })))
const OrderDetailPage = lazy(() => import('@/pages/merchant/orders/DetailPage').then((m) => ({ default: m.DetailPage })))
const AdminReviewDetailPage = lazy(() =>
  import('@/pages/admin/merchants/ReviewDetailPage').then((m) => ({ default: m.ReviewDetailPage }))
)
const AdminLogsPage = lazy(() => import('@/pages/admin/logs/ListPage').then((m) => ({ default: m.ListPage })))
const MerchantLogsPage = lazy(() => import('@/pages/merchant/logs/ListPage').then((m) => ({ default: m.ListPage })))
const AccountPage = lazy(() => import('@/pages/merchant/account/AccountPage').then((m) => ({ default: m.AccountPage })))
const IntentListPage = lazy(() => import('@/pages/merchant/intents/ListPage').then((m) => ({ default: m.ListPage })))
const IntentDetailPage = lazy(() => import('@/pages/merchant/intents/DetailPage').then((m) => ({ default: m.DetailPage })))

function loadable(node: ReactNode) {
  return <Suspense fallback={<p>加载中...</p>}>{node}</Suspense>
}

export function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={loadable(<LoginPage />)} />
        <Route path="/admin/login" element={loadable(<AdminLoginPage />)} />
        <Route path="/register" element={loadable(<RegisterPage />)} />
        <Route element={<RequireAuth role="MERCHANT" />}>
          <Route path="/register/status" element={loadable(<RegisterStatusPage />)} />
        </Route>

        <Route element={<RequireAuth />}>
          <Route element={loadable(<Layout />)}>
            <Route element={<RequireAuth role="ADMIN" />}>
              <Route path="/admin/merchants/reviews" element={loadable(<AdminReviewsPage />)} />
              <Route path="/admin/merchants/reviews/:merchantId" element={loadable(<AdminReviewDetailPage />)} />
              <Route path="/admin/logs" element={loadable(<AdminLogsPage />)} />
            </Route>

            <Route element={<RequireAuth role="MERCHANT" scope="full" />}>
              <Route path="/merchant/dashboard" element={loadable(<DashboardPage />)} />
              <Route path="/merchant/products" element={loadable(<ProductListPage />)} />
              <Route path="/merchant/products/new" element={loadable(<ProductCreatePage />)} />
              <Route path="/merchant/products/:productId" element={loadable(<ProductDetailPage />)} />
              <Route path="/merchant/products/:productId/edit" element={loadable(<ProductEditPage />)} />
              <Route path="/merchant/orders" element={loadable(<OrderListPage />)} />
              <Route path="/merchant/orders/:orderId" element={loadable(<OrderDetailPage />)} />
              <Route path="/merchant/intents" element={loadable(<IntentListPage />)} />
              <Route path="/merchant/intents/:intentId" element={loadable(<IntentDetailPage />)} />
              <Route path="/merchant/account" element={loadable(<AccountPage />)} />
              <Route path="/merchant/logs" element={loadable(<MerchantLogsPage />)} />
            </Route>
          </Route>
        </Route>

        <Route path="*" element={<Navigate to="/login" replace />} />
      </Routes>
    </BrowserRouter>
  )
}
