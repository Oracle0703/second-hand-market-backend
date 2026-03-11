import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { Layout } from './Layout'
import { RequireAuth } from './guards'
import { LoginPage } from '../pages/LoginPage'
import { RegisterPage } from '../pages/RegisterPage'
import { RegisterStatusPage } from '../pages/RegisterStatusPage'
import { AdminMerchantReviewsPage } from '../pages/AdminMerchantReviewsPage'
import { MerchantDashboardPage } from '../pages/MerchantDashboardPage'
import { MerchantProductsPage } from '../pages/MerchantProductsPage'
import { MerchantOrdersPage } from '../pages/MerchantOrdersPage'
import { MerchantProductCreatePage } from '../pages/MerchantProductCreatePage'
import { MerchantProductEditPage } from '../pages/MerchantProductEditPage'
import { MerchantProductDetailPage } from '../pages/MerchantProductDetailPage'
import { MerchantOrderDetailPage } from '../pages/MerchantOrderDetailPage'
import { AdminMerchantReviewDetailPage } from '../pages/AdminMerchantReviewDetailPage'
import { MerchantLogsPage } from '../pages/MerchantLogsPage'
import { MerchantAccountPage } from '../pages/MerchantAccountPage'
import { PlaceholderPage } from '../pages/PlaceholderPage'
import { MerchantIntentsPage } from '../pages/MerchantIntentsPage'
import { MerchantIntentDetailPage } from '../pages/MerchantIntentDetailPage'

export function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/register" element={<RegisterPage />} />
        <Route element={<RequireAuth role="MERCHANT" />}>
          <Route path="/register/status" element={<RegisterStatusPage />} />
        </Route>

        <Route element={<RequireAuth />}>
          <Route element={<Layout />}>
            <Route element={<RequireAuth role="ADMIN" />}>
              <Route path="/admin/merchants/reviews" element={<AdminMerchantReviewsPage />} />
              <Route path="/admin/merchants/reviews/:merchantId" element={<AdminMerchantReviewDetailPage />} />
              <Route path="/admin/logs" element={<PlaceholderPage title="全局操作日志" />} />
            </Route>

            <Route element={<RequireAuth role="MERCHANT" scope="full" />}>
              <Route path="/merchant/dashboard" element={<MerchantDashboardPage />} />
              <Route path="/merchant/products" element={<MerchantProductsPage />} />
              <Route path="/merchant/products/new" element={<MerchantProductCreatePage />} />
              <Route path="/merchant/products/:productId" element={<MerchantProductDetailPage />} />
              <Route path="/merchant/products/:productId/edit" element={<MerchantProductEditPage />} />
              <Route path="/merchant/orders" element={<MerchantOrdersPage />} />
              <Route path="/merchant/orders/:orderId" element={<MerchantOrderDetailPage />} />
              <Route path="/merchant/intents" element={<MerchantIntentsPage />} />
              <Route path="/merchant/intents/:intentId" element={<MerchantIntentDetailPage />} />
              <Route path="/merchant/account" element={<MerchantAccountPage />} />
              <Route path="/merchant/logs" element={<MerchantLogsPage />} />
            </Route>
          </Route>
        </Route>

        <Route path="*" element={<Navigate to="/login" replace />} />
      </Routes>
    </BrowserRouter>
  )
}
