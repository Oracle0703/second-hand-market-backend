import { Suspense, lazy, type ReactNode } from 'react'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { RequireAuth } from './guards'

const Layout = lazy(() => import('./Layout').then((m) => ({ default: m.Layout })))
const LoginPage = lazy(() => import('../pages/LoginPage').then((m) => ({ default: m.LoginPage })))
const RegisterPage = lazy(() => import('../pages/RegisterPage').then((m) => ({ default: m.RegisterPage })))
const RegisterStatusPage = lazy(() => import('../pages/RegisterStatusPage').then((m) => ({ default: m.RegisterStatusPage })))
const AdminMerchantReviewsPage = lazy(() => import('../pages/AdminMerchantReviewsPage').then((m) => ({ default: m.AdminMerchantReviewsPage })))
const MerchantDashboardPage = lazy(() => import('../pages/MerchantDashboardPage').then((m) => ({ default: m.MerchantDashboardPage })))
const MerchantProductsPage = lazy(() => import('../pages/MerchantProductsPage').then((m) => ({ default: m.MerchantProductsPage })))
const MerchantOrdersPage = lazy(() => import('../pages/MerchantOrdersPage').then((m) => ({ default: m.MerchantOrdersPage })))
const MerchantProductCreatePage = lazy(() => import('../pages/MerchantProductCreatePage').then((m) => ({ default: m.MerchantProductCreatePage })))
const MerchantProductEditPage = lazy(() => import('../pages/MerchantProductEditPage').then((m) => ({ default: m.MerchantProductEditPage })))
const MerchantProductDetailPage = lazy(() => import('../pages/MerchantProductDetailPage').then((m) => ({ default: m.MerchantProductDetailPage })))
const MerchantOrderDetailPage = lazy(() => import('../pages/MerchantOrderDetailPage').then((m) => ({ default: m.MerchantOrderDetailPage })))
const AdminMerchantReviewDetailPage = lazy(() => import('../pages/AdminMerchantReviewDetailPage').then((m) => ({ default: m.AdminMerchantReviewDetailPage })))
const AdminLogsPage = lazy(() => import('../pages/AdminLogsPage').then((m) => ({ default: m.AdminLogsPage })))
const MerchantLogsPage = lazy(() => import('../pages/MerchantLogsPage').then((m) => ({ default: m.MerchantLogsPage })))
const MerchantAccountPage = lazy(() => import('../pages/MerchantAccountPage').then((m) => ({ default: m.MerchantAccountPage })))
const MerchantIntentsPage = lazy(() => import('../pages/MerchantIntentsPage').then((m) => ({ default: m.MerchantIntentsPage })))
const MerchantIntentDetailPage = lazy(() => import('../pages/MerchantIntentDetailPage').then((m) => ({ default: m.MerchantIntentDetailPage })))

function loadable(node: ReactNode) {
  return <Suspense fallback={<p>加载中...</p>}>{node}</Suspense>
}

export function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={loadable(<LoginPage />)} />
        <Route path="/register" element={loadable(<RegisterPage />)} />
        <Route element={<RequireAuth role="MERCHANT" />}>
          <Route path="/register/status" element={loadable(<RegisterStatusPage />)} />
        </Route>

        <Route element={<RequireAuth />}>
          <Route element={loadable(<Layout />)}>
            <Route element={<RequireAuth role="ADMIN" />}>
              <Route path="/admin/merchants/reviews" element={loadable(<AdminMerchantReviewsPage />)} />
              <Route path="/admin/merchants/reviews/:merchantId" element={loadable(<AdminMerchantReviewDetailPage />)} />
              <Route path="/admin/logs" element={loadable(<AdminLogsPage />)} />
            </Route>

            <Route element={<RequireAuth role="MERCHANT" scope="full" />}>
              <Route path="/merchant/dashboard" element={loadable(<MerchantDashboardPage />)} />
              <Route path="/merchant/products" element={loadable(<MerchantProductsPage />)} />
              <Route path="/merchant/products/new" element={loadable(<MerchantProductCreatePage />)} />
              <Route path="/merchant/products/:productId" element={loadable(<MerchantProductDetailPage />)} />
              <Route path="/merchant/products/:productId/edit" element={loadable(<MerchantProductEditPage />)} />
              <Route path="/merchant/orders" element={loadable(<MerchantOrdersPage />)} />
              <Route path="/merchant/orders/:orderId" element={loadable(<MerchantOrderDetailPage />)} />
              <Route path="/merchant/intents" element={loadable(<MerchantIntentsPage />)} />
              <Route path="/merchant/intents/:intentId" element={loadable(<MerchantIntentDetailPage />)} />
              <Route path="/merchant/account" element={loadable(<MerchantAccountPage />)} />
              <Route path="/merchant/logs" element={loadable(<MerchantLogsPage />)} />
            </Route>
          </Route>
        </Route>

        <Route path="*" element={<Navigate to="/login" replace />} />
      </Routes>
    </BrowserRouter>
  )
}
