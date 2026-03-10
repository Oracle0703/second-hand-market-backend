#!/usr/bin/env node

const baseURL = process.env.API_BASE_URL || 'http://localhost:8080/api/v1'

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
    console.error('[SMOKE][FAIL]', msg)
    if (payload) console.error(JSON.stringify(payload, null, 2))
    process.exit(1)
  }
}

function nowSeed() {
  return Date.now().toString().slice(-8)
}

async function main() {
  const seed = nowSeed()
  const username = `smoke_${seed}`
  const password = 'Passw0rd!2026'

  const licPresign = await req('/files/presign', {
    method: 'POST',
    body: { biz_type: 'MERCHANT_LICENSE', file_name: `license-${seed}.jpg`, file_size: 1000, mime_type: 'image/jpeg' }
  })
  assert(licPresign.code === 0, 'license presign failed', licPresign)

  const register = await req('/auth/register', {
    method: 'POST',
    body: {
      merchant_name: `冒烟商家${seed}`,
      contact_name: '冒烟',
      phone: `139${seed}`,
      username,
      password,
      license_file_id: licPresign.data.file_id
    }
  })
  assert(register.code === 0, 'register failed', register)
  const merchantID = register.data.merchant_id

  const pendingLogin = await req('/auth/login', {
    method: 'POST',
    body: { login_type: 'MERCHANT', username, password }
  })
  assert(pendingLogin.code === 0, 'pending login failed', pendingLogin)
  assert(pendingLogin.data.token_scope === 'onboarding', 'pending login should be onboarding scope', pendingLogin)
  const pendingToken = pendingLogin.data.access_token

  const deniedProducts = await req('/merchant/products', {
    headers: { Authorization: `Bearer ${pendingToken}` }
  })
  assert(deniedProducts.code === 10006, 'onboarding token should deny products API', deniedProducts)

  const adminLogin = await req('/auth/login', {
    method: 'POST',
    body: { login_type: 'ADMIN', username: 'admin', password: 'Admin@123456' }
  })
  assert(adminLogin.code === 0, 'admin login failed', adminLogin)
  const adminToken = adminLogin.data.access_token

  const approve = await req(`/admin/merchants/${merchantID}/approve`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${adminToken}` },
    body: { comment: 'smoke approve' }
  })
  assert(approve.code === 0, 'approve failed', approve)

  const fullLogin = await req('/auth/login', {
    method: 'POST',
    body: { login_type: 'MERCHANT', username, password }
  })
  assert(fullLogin.code === 0 && fullLogin.data.token_scope === 'full', 'full login failed', fullLogin)
  const merchantToken = fullLogin.data.access_token

  const imgPresign = await req('/files/presign', {
    method: 'POST',
    headers: { Authorization: `Bearer ${merchantToken}` },
    body: { biz_type: 'PRODUCT_IMAGE', file_name: `product-${seed}.jpg`, file_size: 1000, mime_type: 'image/jpeg' }
  })
  assert(imgPresign.code === 0, 'product image presign failed', imgPresign)

  const categories = await req('/merchant/categories?level=2', {
    headers: { Authorization: `Bearer ${merchantToken}` }
  })
  assert(categories.code === 0 && categories.data.items?.length > 0, 'load categories failed', categories)
  const categoryID = categories.data.items[0].id || categories.data.items[0].ID

  const createProduct = await req('/merchant/products', {
    method: 'POST',
    headers: { Authorization: `Bearer ${merchantToken}` },
    body: {
      title: `smoke-product-${seed}`,
      description: 'smoke',
      category_id: categoryID,
      price_cent: 12345,
      condition_level: 'GOOD',
      stock: 1,
      image_file_ids: [imgPresign.data.file_id]
    }
  })
  assert(createProduct.code === 0, 'create product failed', createProduct)
  const productID = createProduct.data.product_id

  const onShelf = await req(`/merchant/products/${productID}/on-shelf`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${merchantToken}` },
    body: {}
  })
  assert(onShelf.code === 0, 'on shelf failed', onShelf)

  const createOrder = await req('/merchant/orders', {
    method: 'POST',
    headers: { Authorization: `Bearer ${merchantToken}` },
    body: { product_id: productID, deal_price_cent: 12000 }
  })
  assert(createOrder.code === 0, 'create order failed', createOrder)
  const orderID = createOrder.data.order_id

  const completeOrder = await req(`/merchant/orders/${orderID}/complete`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${merchantToken}` },
    body: { note: 'smoke complete' }
  })
  assert(completeOrder.code === 0, 'complete order failed', completeOrder)

  const soldDetail = await req(`/merchant/products/${productID}`, {
    headers: { Authorization: `Bearer ${merchantToken}` }
  })
  assert(soldDetail.code === 0 && soldDetail.data.product?.status === 'SOLD', 'completed order should set product SOLD', soldDetail)

  const createProduct2 = await req('/merchant/products', {
    method: 'POST',
    headers: { Authorization: `Bearer ${merchantToken}` },
    body: {
      title: `smoke-product-close-${seed}`,
      description: 'smoke close flow',
      category_id: categoryID,
      price_cent: 12500,
      condition_level: 'GOOD',
      stock: 1,
      image_file_ids: [imgPresign.data.file_id]
    }
  })
  assert(createProduct2.code === 0, 'create product for close flow failed', createProduct2)
  const product2ID = createProduct2.data.product_id

  const onShelf2 = await req(`/merchant/products/${product2ID}/on-shelf`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${merchantToken}` },
    body: {}
  })
  assert(onShelf2.code === 0, 'second on shelf failed', onShelf2)

  const createOrder2 = await req('/merchant/orders', {
    method: 'POST',
    headers: { Authorization: `Bearer ${merchantToken}` },
    body: { product_id: product2ID, deal_price_cent: 12100 }
  })
  assert(createOrder2.code === 0, 'create second order failed', createOrder2)
  const order2ID = createOrder2.data.order_id

  const closeOrder = await req(`/merchant/orders/${order2ID}/close`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${merchantToken}` },
    body: { reason: 'smoke close' }
  })
  assert(closeOrder.code === 0, 'close order failed', closeOrder)

  const closedProductDetail = await req(`/merchant/products/${product2ID}`, {
    headers: { Authorization: `Bearer ${merchantToken}` }
  })
  assert(
    closedProductDetail.code === 0 && closedProductDetail.data.product?.status === 'OFF_SHELF',
    'closed order should set product OFF_SHELF',
    closedProductDetail
  )

  console.log('[SMOKE][PASS] restricted login + 审核 + 商品 + 订单主链路通过')
}

main().catch((err) => {
  console.error('[SMOKE][ERROR]', err)
  process.exit(1)
})
