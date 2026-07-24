#!/usr/bin/env node

import { randomBytes } from 'node:crypto'

const baseURL = process.env.API_BASE_URL || 'http://localhost:8080/api/v1'
const deviceId = process.env.BUYER_DEVICE_ID || 'smoke-buyer-device-001'

async function req(path, { method = 'GET', headers = {}, body } = {}) {
  const res = await fetch(baseURL + path, {
    method,
    headers: {
      'Content-Type': 'application/json',
      ...headers
    },
    body: body ? JSON.stringify(body) : undefined
  })
  const json = await res.json()
  return json
}

function assert(ok, msg, payload) {
  if (!ok) {
    console.error('[SMOKE-BUYER][FAIL]', msg)
    if (payload) console.error(JSON.stringify(payload, null, 2))
    process.exit(1)
  }
}

function nowSeed() {
  return Date.now().toString().slice(-8)
}

function requiredEnv(name) {
  const value = String(process.env[name] || '').trim()
  if (!value) throw new Error(`${name} is required`)
  return value
}

function randomSmokePassword() {
  return `Sm0ke!${randomBytes(18).toString('base64url')}`
}

async function setupMerchantAndProduct(seed, adminUsername, adminPassword) {
  const username = `smoke_buyer_${seed}`
  const password = randomSmokePassword()

  const lic = await req('/files/presign', {
    method: 'POST',
    body: { biz_type: 'MERCHANT_LICENSE', file_name: `license-${seed}.jpg`, file_size: 1000, mime_type: 'image/jpeg' }
  })
  assert(lic.code === 0, 'merchant license presign failed', lic)

  const register = await req('/auth/register', {
    method: 'POST',
    body: {
      merchant_name: `买家冒烟商家${seed}`,
      contact_name: '测试商家',
      phone: `139${seed}`,
      username,
      password,
      license_file_id: lic.data.file_id
    }
  })
  assert(register.code === 0, 'merchant register failed', register)

  const adminLogin = await req('/auth/login', {
    method: 'POST',
    body: { login_type: 'ADMIN', username: adminUsername, password: adminPassword }
  })
  assert(adminLogin.code === 0, 'admin login failed', adminLogin)
  const adminToken = adminLogin.data.access_token

  const approve = await req(`/admin/merchants/${register.data.merchant_id}/approve`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${adminToken}` },
    body: { comment: 'smoke buyer approve' }
  })
  assert(approve.code === 0, 'merchant approve failed', approve)

  const merchantLogin = await req('/auth/login', {
    method: 'POST',
    body: { login_type: 'MERCHANT', username, password }
  })
  assert(merchantLogin.code === 0 && merchantLogin.data.token_scope === 'full', 'merchant login(full) failed', merchantLogin)
  const merchantToken = merchantLogin.data.access_token

  const image = await req('/files/presign', {
    method: 'POST',
    headers: { Authorization: `Bearer ${merchantToken}` },
    body: { biz_type: 'PRODUCT_IMAGE', file_name: `product-${seed}.jpg`, file_size: 1000, mime_type: 'image/jpeg' }
  })
  assert(image.code === 0, 'product image presign failed', image)

  const categories = await req('/merchant/categories?level=2', {
    headers: { Authorization: `Bearer ${merchantToken}` }
  })
  assert(categories.code === 0 && categories.data.items?.length > 0, 'load category failed', categories)
  const categoryID = categories.data.items[0].id || categories.data.items[0].ID

  const createProduct = await req('/merchant/products', {
    method: 'POST',
    headers: { Authorization: `Bearer ${merchantToken}` },
    body: {
      title: `smoke-buyer-product-${seed}`,
      description: 'smoke buyer flow product',
      category_id: categoryID,
      price_cent: 45600,
      original_price_cent: 49900,
      condition_level: 'GOOD',
      stock: 1,
      image_file_ids: [image.data.file_id]
    }
  })
  assert(createProduct.code === 0, 'create product failed', createProduct)
  const productID = createProduct.data.product_id

  const onShelf = await req(`/merchant/products/${productID}/on-shelf`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${merchantToken}` },
    body: {}
  })
  assert(onShelf.code === 0, 'product on shelf failed', onShelf)

  return { productID, merchantToken }
}

async function main() {
  const seed = nowSeed()
  const adminUsername = requiredEnv('SMOKE_ADMIN_USERNAME')
  const adminPassword = requiredEnv('SMOKE_ADMIN_PASSWORD')
  const { productID, merchantToken } = await setupMerchantAndProduct(seed, adminUsername, adminPassword)

  const guestList = await req('/buyer/products?page=1&page_size=10', {
    headers: { 'X-Device-Id': deviceId }
  })
  assert(guestList.code === 0, '1) guest list failed', guestList)

  const guestDetail = await req(`/buyer/products/${productID}`, {
    headers: { 'X-Device-Id': deviceId }
  })
  assert(guestDetail.code === 0, '2) guest detail failed', guestDetail)

  const guestFavorite = await req('/buyer/favorites', {
    method: 'POST',
    headers: { 'X-Device-Id': deviceId },
    body: { product_id: productID }
  })
  assert(guestFavorite.code === 0, '3) guest favorite failed', guestFavorite)

  const buyerLogin = await req('/buyer/auth/wechat-login', {
    method: 'POST',
    body: { code: `smoke-wx-${seed}`, device_id: deviceId, nickname: 'smoke-buyer' }
  })
  assert(buyerLogin.code === 0, '4) buyer login failed', buyerLogin)
  const buyerToken = buyerLogin.data.access_token

  const merge = await req('/buyer/guest/merge', {
    method: 'POST',
    headers: { Authorization: `Bearer ${buyerToken}` },
    body: { device_id: deviceId }
  })
  assert(merge.code === 0, '5) guest merge failed', merge)

  const createIntent = await req('/buyer/intents', {
    method: 'POST',
    headers: { Authorization: `Bearer ${buyerToken}`, 'X-Device-Id': deviceId },
    body: { product_id: productID, contact_phone: '13800138000', message: 'smoke intent' }
  })
  assert(createIntent.code === 0, '6) buyer create intent failed', createIntent)
  const intentID = createIntent.data.intent_id

  const merchantList = await req('/merchant/intents?page=1&page_size=10', {
    headers: { Authorization: `Bearer ${merchantToken}` }
  })
  assert(merchantList.code === 0, '7) merchant intent list failed', merchantList)

  const merchantContacted = await req(`/merchant/intents/${intentID}/contacted`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${merchantToken}` },
    body: {}
  })
  assert(merchantContacted.code === 0, '8) merchant mark contacted failed', merchantContacted)

  const buyerIntents = await req('/buyer/intents?page=1&page_size=10', {
    headers: { Authorization: `Bearer ${buyerToken}` }
  })
  assert(buyerIntents.code === 0, '9) buyer intent list failed', buyerIntents)
  const first = buyerIntents.data.items?.[0]
  assert(first && first.buyer_status_text === '已联系', '9) buyer status should be 已联系', buyerIntents)

  console.log('[SMOKE-BUYER][PASS] 游客浏览 -> 收藏 -> 登录 -> merge -> 提交意向 -> 商家处理 -> 买家回看 状态闭环通过')
}

main().catch((err) => {
  console.error('[SMOKE-BUYER][ERROR]', err)
  process.exit(1)
})
