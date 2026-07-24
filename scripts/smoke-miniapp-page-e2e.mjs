#!/usr/bin/env node

import { randomBytes } from 'node:crypto'

const baseURL = process.env.API_BASE_URL || 'http://localhost:8080/api/v1'

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

const seed = nowSeed()
const deviceId = process.env.BUYER_DEVICE_ID || `smoke-miniapp-device-${seed}`
const wechatLoginCode = process.env.BUYER_WECHAT_LOGIN_CODE || `miniapp-page-${seed}`
const healthURL = `${baseURL.replace(/\/api\/v1\/?$/, '')}/healthz`

async function req(path, { method = 'GET', headers = {}, body } = {}) {
  try {
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
  } catch (err) {
    throw new Error(`request failed: ${baseURL + path}; ensure backend is reachable (${String(err)})`)
  }
}

function assert(ok, msg, payload) {
  if (!ok) {
    console.error('[SMOKE-MINIAPP-PAGE][FAIL]', msg)
    if (payload) {
      console.error(JSON.stringify(payload, null, 2))
    }
    process.exit(1)
  }
}

function step(title) {
  console.log(`[SMOKE-MINIAPP-PAGE][STEP] ${title}`)
}

async function setupMerchantAndProduct(runSeed, adminUsername, adminPassword) {
  const username = `smoke_page_${runSeed}`
  const password = randomSmokePassword()

  const license = await req('/files/presign', {
    method: 'POST',
    body: {
      biz_type: 'MERCHANT_LICENSE',
      file_name: `license-${runSeed}.jpg`,
      file_size: 1024,
      mime_type: 'image/jpeg'
    }
  })
  assert(license.code === 0, 'merchant license presign failed', license)

  const register = await req('/auth/register', {
    method: 'POST',
    body: {
      merchant_name: `页面冒烟商家${runSeed}`,
      contact_name: '页面冒烟',
      phone: `139${runSeed}`,
      username,
      password,
      license_file_id: license.data.file_id
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
    body: { comment: 'smoke miniapp page e2e approve' }
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
    body: {
      biz_type: 'PRODUCT_IMAGE',
      file_name: `product-${runSeed}.jpg`,
      file_size: 1024,
      mime_type: 'image/jpeg'
    }
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
      title: `smoke-miniapp-page-product-${runSeed}`,
      description: 'miniapp page e2e smoke product',
      category_id: categoryID,
      price_cent: 56800,
      original_price_cent: 59900,
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

  return { productID, categoryID, merchantToken }
}

function findByProduct(items, productID, field = 'id') {
  return (items || []).find((it) => Number(it[field]) === Number(productID))
}

async function main() {
  const adminUsername = requiredEnv('SMOKE_ADMIN_USERNAME')
  const adminPassword = requiredEnv('SMOKE_ADMIN_PASSWORD')
  step('0/7 联调前健康检查')
  let health
  try {
    const res = await fetch(healthURL)
    health = await res.json()
  } catch (err) {
    throw new Error(`health check failed: ${healthURL}; ensure backend is reachable (${String(err)})`)
  }
  assert(health.code === 0, 'health check failed', health)

  const { productID, categoryID, merchantToken } = await setupMerchantAndProduct(seed, adminUsername, adminPassword)

  step('1/7 首页 -> 商品列表 -> 商品详情（游客）')
  const homeList = await req('/buyer/products?page=1&page_size=50&sort=latest', {
    headers: { 'X-Device-Id': deviceId }
  })
  assert(homeList.code === 0, 'guest home list failed', homeList)
  assert(!!findByProduct(homeList.data.items, productID), 'product not found in guest home list', homeList)

  const categoryList = await req(`/buyer/products?page=1&page_size=50&category_id=${categoryID}`, {
    headers: { 'X-Device-Id': deviceId }
  })
  assert(categoryList.code === 0, 'guest product list by category failed', categoryList)
  assert(!!findByProduct(categoryList.data.items, productID), 'product not found in category list', categoryList)

  const detail = await req(`/buyer/products/${productID}`, {
    headers: { 'X-Device-Id': deviceId }
  })
  assert(detail.code === 0, 'guest product detail failed', detail)
  assert(Number(detail.data.product?.id) === Number(productID), 'product detail id mismatch', detail)

  step('2/7 游客收藏商品')
  const addFavorite = await req('/buyer/favorites', {
    method: 'POST',
    headers: { 'X-Device-Id': deviceId },
    body: { product_id: productID }
  })
  assert(addFavorite.code === 0, 'guest add favorite failed', addFavorite)

  const guestFavorites = await req('/buyer/favorites?page=1&page_size=20', {
    headers: { 'X-Device-Id': deviceId }
  })
  assert(guestFavorites.code === 0, 'guest favorite list failed', guestFavorites)
  assert(!!findByProduct(guestFavorites.data.items, productID, 'product_id'), 'favorite product not found in guest list', guestFavorites)

  step('3/7 游客浏览记录生成')
  const report = await req('/buyer/histories/views', {
    method: 'POST',
    headers: { 'X-Device-Id': deviceId },
    body: { product_id: productID }
  })
  assert(report.code === 0, 'guest report view failed', report)

  const guestHistories = await req('/buyer/histories?page=1&page_size=20', {
    headers: { 'X-Device-Id': deviceId }
  })
  assert(guestHistories.code === 0, 'guest history list failed', guestHistories)
  const guestHistoryRow = findByProduct(guestHistories.data.items, productID, 'product_id')
  assert(!!guestHistoryRow, 'history product not found in guest list', guestHistories)
  assert(Number(guestHistoryRow.view_count) >= 1, 'history view_count should be >= 1', guestHistories)

  step('4/7 登录后 guest merge（含游客提交意向拦截）')
  const guestIntent = await req('/buyer/intents', {
    method: 'POST',
    headers: { 'X-Device-Id': deviceId },
    body: { product_id: productID, contact_phone: '13800138000', message: 'guest should fail' }
  })
  assert(guestIntent.code === 10002, 'guest intent create should be blocked', guestIntent)

  const login = await req('/buyer/auth/wechat-login', {
    method: 'POST',
    body: { code: wechatLoginCode, device_id: deviceId, nickname: 'miniapp-page-smoke-buyer' }
  })
  assert(
    login.code === 0,
    'buyer login failed (if backend is in real mode, pass BUYER_WECHAT_LOGIN_CODE from wx.login)',
    login
  )
  const buyerToken = login.data.access_token

  const merge = await req('/buyer/guest/merge', {
    method: 'POST',
    headers: { Authorization: `Bearer ${buyerToken}` },
    body: { device_id: deviceId }
  })
  assert(merge.code === 0, 'guest merge failed', merge)

  const summary = await req('/buyer/me/summary', {
    headers: { Authorization: `Bearer ${buyerToken}`, 'X-Device-Id': deviceId }
  })
  assert(summary.code === 0, 'buyer summary failed', summary)
  assert(summary.data.is_login === true, 'buyer should be login state', summary)
  assert(Number(summary.data.counters?.favorites) >= 1, 'favorites should be merged to buyer', summary)
  assert(Number(summary.data.counters?.histories) >= 1, 'histories should be merged to buyer', summary)

  step('5/7 登录用户提交购买意向')
  const createIntent = await req('/buyer/intents', {
    method: 'POST',
    headers: { Authorization: `Bearer ${buyerToken}`, 'X-Device-Id': deviceId },
    body: { product_id: productID, contact_phone: '13800138000', message: 'miniapp page e2e intent' }
  })
  assert(createIntent.code === 0, 'buyer create intent failed', createIntent)
  const intentID = createIntent.data.intent_id

  step('6/7 我的意向列表查看状态（NEW -> 处理中）')
  const buyerIntentList = await req('/buyer/intents?page=1&page_size=20', {
    headers: { Authorization: `Bearer ${buyerToken}` }
  })
  assert(buyerIntentList.code === 0, 'buyer intent list failed', buyerIntentList)
  const createdIntent = findByProduct(
    (buyerIntentList.data.items || []).map((it) => ({ ...it, product_id: it.product?.id })),
    productID,
    'product_id'
  )
  assert(!!createdIntent, 'created intent not found in buyer list', buyerIntentList)
  assert(createdIntent.status === 'NEW', 'new intent status should be NEW', buyerIntentList)
  assert(createdIntent.buyer_status_text === '处理中', 'new intent buyer_status_text should be 处理中', buyerIntentList)

  step('7/7 商家处理意向后，买家端状态回读（CONTACTED -> CLOSED）')
  const merchantContacted = await req(`/merchant/intents/${intentID}/contacted`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${merchantToken}` },
    body: {}
  })
  assert(merchantContacted.code === 0, 'merchant mark contacted failed', merchantContacted)

  const buyerIntentAfterContacted = await req(`/buyer/intents/${intentID}`, {
    headers: { Authorization: `Bearer ${buyerToken}` }
  })
  assert(buyerIntentAfterContacted.code === 0, 'buyer intent detail after contacted failed', buyerIntentAfterContacted)
  assert(buyerIntentAfterContacted.data.intent?.status === 'CONTACTED', 'intent status should be CONTACTED', buyerIntentAfterContacted)
  assert(buyerIntentAfterContacted.data.intent?.buyer_status_text === '已联系', 'buyer status should be 已联系', buyerIntentAfterContacted)

  const merchantClose = await req(`/merchant/intents/${intentID}/close`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${merchantToken}` },
    body: { reason: 'NO_RESPONSE', merchant_note: 'miniapp page e2e close' }
  })
  assert(merchantClose.code === 0, 'merchant close intent failed', merchantClose)

  const buyerClosedList = await req('/buyer/intents?status=CLOSED&page=1&page_size=20', {
    headers: { Authorization: `Bearer ${buyerToken}` }
  })
  assert(buyerClosedList.code === 0, 'buyer closed intent list failed', buyerClosedList)
  const closedIntent = (buyerClosedList.data.items || []).find((it) => Number(it.id) === Number(intentID))
  assert(!!closedIntent, 'closed intent not found in buyer closed list', buyerClosedList)
  assert(closedIntent.buyer_status_text === '已关闭', 'closed intent buyer_status_text should be 已关闭', buyerClosedList)

  console.log('[SMOKE-MINIAPP-PAGE][PASS] 页面链路：浏览 -> 收藏 -> 浏览记录 -> 登录合并 -> 提交意向 -> 商家处理 -> 买家状态回读 全链路通过')
}

main().catch((err) => {
  console.error('[SMOKE-MINIAPP-PAGE][ERROR]', err)
  process.exit(1)
})
