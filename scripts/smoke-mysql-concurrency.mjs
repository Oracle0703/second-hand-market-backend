#!/usr/bin/env node

/**
 * Destructive acceptance smoke for an isolated MySQL 8.4 environment.
 *
 * Required environment variables:
 *   API_BASE_URL=http://127.0.0.1:18082/api/v1
 *   ACCEPTANCE_DB_ENGINE=mysql8.4
 *   ACCEPTANCE_CONFIRM_ISOLATED=I_UNDERSTAND_THIS_WRITES_TEST_DATA
 *   SMOKE_ADMIN_USERNAME=<injected secret>
 *   SMOKE_ADMIN_PASSWORD=<injected secret>
 *
 * The target URL must use a loopback host. The script creates a merchant,
 * file records, products, orders, sessions, audit rows, and operation logs.
 * It best-effort closes active test orders, closes test products, and logs
 * out its sessions, but it does not delete database rows. Recreate the
 * isolated database/volume for complete cleanup. Never run this on production.
 *
 * This script validates behavior through the API. Migration, CHECK constraint,
 * and SHOW INDEX assertions still need the companion SQL acceptance checks.
 */

import { randomBytes } from 'node:crypto'

const CODE = {
  OK: 0,
  INVALID_ARGUMENT: 10001,
  INVALID_TRANSITION: 10005,
  CONFLICT: 10010
}

const MAX_DB_INT = 2147483647
const ISOLATED_CONFIRMATION = 'I_UNDERSTAND_THIS_WRITES_TEST_DATA'
const LOOPBACK_HOSTS = new Set(['127.0.0.1', 'localhost', '[::1]'])
const REQUEST_TIMEOUT_MS = 20_000

let baseURL = ''
let adminToken = ''
let merchantToken = ''
const productIDs = new Set()

function requiredEnv(name) {
  const value = String(process.env[name] || '').trim()
  if (!value) throw new Error(`${name} is required`)
  return value
}

function configureTarget() {
  const rawBaseURL = requiredEnv('API_BASE_URL')
  const parsed = new URL(rawBaseURL)
  if (!LOOPBACK_HOSTS.has(parsed.hostname)) {
    throw new Error('API_BASE_URL must use localhost/loopback and reach acceptance through an SSH tunnel or local binding')
  }
  if (!/\/api\/v1\/?$/.test(parsed.pathname)) {
    throw new Error('API_BASE_URL must end with /api/v1')
  }
  if (requiredEnv('ACCEPTANCE_DB_ENGINE').toLowerCase() !== 'mysql8.4') {
    throw new Error('ACCEPTANCE_DB_ENGINE must be mysql8.4')
  }
  if (requiredEnv('ACCEPTANCE_CONFIRM_ISOLATED') !== ISOLATED_CONFIRMATION) {
    throw new Error(`ACCEPTANCE_CONFIRM_ISOLATED must equal ${ISOLATED_CONFIRMATION}`)
  }
  baseURL = rawBaseURL.replace(/\/$/, '')
}

function redact(value) {
  if (Array.isArray(value)) return value.map(redact)
  if (!value || typeof value !== 'object') return value
  return Object.fromEntries(
    Object.entries(value).map(([key, item]) => {
      const normalized = key.toLowerCase()
      if (normalized.includes('password') || normalized.includes('token') || normalized.includes('secret')) {
        return [key, '[REDACTED]']
      }
      return [key, redact(item)]
    })
  )
}

function fail(message, payload) {
  const detail = payload === undefined ? '' : `\n${JSON.stringify(redact(payload), null, 2)}`
  throw new Error(`${message}${detail}`)
}

function assert(condition, message, payload) {
  if (!condition) fail(message, payload)
}

function step(message) {
  console.log(`[MYSQL-ACCEPTANCE][STEP] ${message}`)
}

function uniqueRunID() {
  return `${Date.now().toString(36)}_${randomBytes(6).toString('hex')}`
}

function randomSmokePassword() {
  return `Sm0ke!${randomBytes(24).toString('base64url')}`
}

function authorization(token, extra = {}) {
  return { Authorization: `Bearer ${token}`, ...extra }
}

async function req(path, { method = 'GET', headers = {}, body } = {}) {
  let response
  try {
    response = await fetch(baseURL + path, {
      method,
      headers: {
        'Content-Type': 'application/json',
        ...headers
      },
      body: body === undefined ? undefined : JSON.stringify(body),
      signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS)
    })
  } catch (error) {
    throw new Error(`request failed for ${method} ${path}: ${error instanceof Error ? error.message : String(error)}`)
  }

  try {
    return await response.json()
  } catch {
    throw new Error(`non-JSON response for ${method} ${path} (HTTP ${response.status})`)
  }
}

async function runConcurrently(count, operation) {
  let release
  const start = new Promise((resolve) => {
    release = resolve
  })
  const tasks = Array.from({ length: count }, (_, index) => (async () => {
    await start
    return operation(index)
  })())
  release()
  const results = await Promise.allSettled(tasks)
  const failures = results.filter((result) => result.status === 'rejected')
  if (failures.length > 0) {
    const reasons = failures.map((result) => result.reason instanceof Error ? result.reason.message : String(result.reason))
    throw new Error(`concurrent requests failed after all requests settled: ${reasons.join('; ')}`)
  }
  return results.map((result) => result.value)
}

async function healthCheck() {
  const healthURL = `${baseURL.replace(/\/api\/v1$/, '')}/healthz`
  let response
  try {
    response = await fetch(healthURL, { signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS) })
  } catch (error) {
    throw new Error(`health check failed for ${healthURL}: ${error instanceof Error ? error.message : String(error)}`)
  }
  let json
  try {
    json = await response.json()
  } catch {
    throw new Error(`health check returned non-JSON data (HTTP ${response.status})`)
  }
  assert(json.code === CODE.OK, 'health check returned a failure', json)
}

async function setupMerchant(runID, adminUsername, adminPassword) {
  const username = `mysql_accept_${runID}`
  const password = randomSmokePassword()
  const phoneSuffix = String(randomBytes(4).readUInt32BE() % 100_000_000).padStart(8, '0')

  const license = await req('/files/presign', {
    method: 'POST',
    body: {
      biz_type: 'MERCHANT_LICENSE',
      file_name: `mysql-accept-license-${runID}.jpg`,
      file_size: 1024,
      mime_type: 'image/jpeg'
    }
  })
  assert(license.code === CODE.OK, 'license presign failed', license)

  const register = await req('/auth/register', {
    method: 'POST',
    body: {
      merchant_name: `MySQL Acceptance ${runID}`,
      contact_name: 'Acceptance Operator',
      phone: `139${phoneSuffix}`,
      username,
      password,
      license_file_id: license.data.file_id
    }
  })
  assert(register.code === CODE.OK, 'merchant registration failed', register)

  const adminLogin = await req('/auth/login', {
    method: 'POST',
    body: { login_type: 'ADMIN', username: adminUsername, password: adminPassword }
  })
  assert(adminLogin.code === CODE.OK, 'admin login failed', adminLogin)
  adminToken = adminLogin.data.access_token

  const approve = await req(`/admin/merchants/${register.data.merchant_id}/approve`, {
    method: 'POST',
    headers: authorization(adminToken),
    body: { comment: `isolated MySQL acceptance ${runID}` }
  })
  assert(approve.code === CODE.OK, 'merchant approval failed', approve)

  const merchantLogin = await req('/auth/login', {
    method: 'POST',
    body: { login_type: 'MERCHANT', username, password }
  })
  assert(
    merchantLogin.code === CODE.OK && merchantLogin.data.token_scope === 'full',
    'approved merchant login failed',
    merchantLogin
  )
  merchantToken = merchantLogin.data.access_token

  const categories = await req('/merchant/categories?level=2', {
    headers: authorization(merchantToken)
  })
  assert(categories.code === CODE.OK && categories.data.items?.length > 0, 'no level-2 category is available', categories)

  return {
    categoryID: categories.data.items[0].id || categories.data.items[0].ID,
    merchantID: register.data.merchant_id,
    username
  }
}

async function createOnShelfProduct(runID, categoryID, label, stock) {
  const image = await req('/files/presign', {
    method: 'POST',
    headers: authorization(merchantToken),
    body: {
      biz_type: 'PRODUCT_IMAGE',
      file_name: `mysql-accept-${label}-${runID}.jpg`,
      file_size: 1024,
      mime_type: 'image/jpeg'
    }
  })
  assert(image.code === CODE.OK, `image presign failed for ${label}`, image)

  const created = await req('/merchant/products', {
    method: 'POST',
    headers: authorization(merchantToken),
    body: {
      title: `mysql-accept-${label}-${runID}`,
      description: `isolated MySQL acceptance product for ${label}`,
      category_id: categoryID,
      price_cent: 10000,
      original_price_cent: 12000,
      condition_level: 'GOOD',
      stock,
      image_file_ids: [image.data.file_id]
    }
  })
  assert(created.code === CODE.OK, `product creation failed for ${label}`, created)
  const productID = Number(created.data.product_id)
  productIDs.add(productID)

  const onShelf = await req(`/merchant/products/${productID}/on-shelf`, {
    method: 'POST',
    headers: authorization(merchantToken),
    body: {}
  })
  assert(onShelf.code === CODE.OK, `on-shelf failed for ${label}`, onShelf)
  return productID
}

async function productDetail(productID) {
  const detail = await req(`/merchant/products/${productID}`, {
    headers: authorization(merchantToken)
  })
  assert(detail.code === CODE.OK, `product detail failed for ${productID}`, detail)
  return detail.data.product
}

async function orderDetail(orderID) {
  const detail = await req(`/merchant/orders/${orderID}`, {
    headers: authorization(merchantToken)
  })
  assert(detail.code === CODE.OK, `order detail failed for ${orderID}`, detail)
  return detail.data.order_detail
}

async function createOrder(productID, quantity, dealPriceCent, suffix) {
  return req('/merchant/orders', {
    method: 'POST',
    headers: authorization(merchantToken),
    body: {
      product_id: productID,
      quantity,
      deal_price_cent: dealPriceCent,
      remark: `mysql acceptance ${suffix}`
    }
  })
}

async function completeOrder(orderID, idempotencyKey, note = 'mysql acceptance complete') {
  return req(`/merchant/orders/${orderID}/complete`, {
    method: 'POST',
    headers: authorization(merchantToken, { 'Idempotency-Key': idempotencyKey }),
    body: { note }
  })
}

async function closeOrder(orderID, idempotencyKey, reason = 'mysql acceptance close') {
  return req(`/merchant/orders/${orderID}/close`, {
    method: 'POST',
    headers: authorization(merchantToken, { 'Idempotency-Key': idempotencyKey }),
    body: { reason }
  })
}

function assertInventory(product, expected, context) {
  const actual = {
    status: product?.status,
    stock: Number(product?.stock),
    reservedStock: Number(product?.reserved_stock),
    availableStock: Number(product?.available_stock)
  }
  assert(
    actual.status === expected.status &&
      actual.stock === expected.stock &&
      actual.reservedStock === expected.reservedStock &&
      actual.availableStock === expected.availableStock,
    `${context}: inventory mismatch`,
    { expected, actual }
  )
}

async function activeOrdersForProduct(productID) {
  const list = await req('/merchant/orders?status=CREATED&page=1&page_size=100', {
    headers: authorization(merchantToken)
  })
  assert(list.code === CODE.OK, 'active order list failed', list)
  return (list.data.items || []).filter((item) => Number(item.product_id) === Number(productID))
}

async function testInputBoundaries(runID, categoryID) {
  step('1/5 quantity and unit-price boundaries')
  const productID = await createOnShelfProduct(runID, categoryID, 'boundaries', 5)
  const invalidCases = [
    { quantity: 0, price: 1000, label: 'zero quantity' },
    { quantity: -1, price: 1000, label: 'negative quantity' },
    { quantity: MAX_DB_INT + 1, price: 1000, label: 'quantity above signed INT' },
    { quantity: 1, price: 0, label: 'zero unit price' },
    { quantity: 1, price: -1, label: 'negative unit price' },
    { quantity: 1, price: MAX_DB_INT + 1, label: 'unit price above signed INT' }
  ]
  for (const testCase of invalidCases) {
    const response = await createOrder(productID, testCase.quantity, testCase.price, testCase.label)
    assert(response.code === CODE.INVALID_ARGUMENT, `${testCase.label} must be rejected`, response)
  }

  const insufficient = await createOrder(productID, 6, 1000, 'insufficient stock')
  assert(insufficient.code === CODE.CONFLICT, 'quantity above available stock must conflict', insufficient)
  assertInventory(await productDetail(productID), {
    status: 'ON_SHELF', stock: 5, reservedStock: 0, availableStock: 5
  }, 'invalid requests')

  const maxProductID = await createOnShelfProduct(runID, categoryID, 'max-quantity', MAX_DB_INT)
  const maxOrder = await createOrder(maxProductID, MAX_DB_INT, 1, 'signed INT quantity maximum')
  assert(
    maxOrder.code === CODE.OK &&
      Number(maxOrder.data.quantity) === MAX_DB_INT &&
      Number(maxOrder.data.deal_price_cent) === 1 &&
      Number(maxOrder.data.total_deal_price_cent) === MAX_DB_INT,
    'signed INT quantity maximum should produce an exact safe-integer total',
    maxOrder
  )
  const maxClose = await closeOrder(maxOrder.data.order_id, `${runID}-max-close`)
  assert(maxClose.code === CODE.OK, 'max-quantity order close failed', maxClose)
  assertInventory(await productDetail(maxProductID), {
    status: 'ON_SHELF', stock: MAX_DB_INT, reservedStock: 0, availableStock: MAX_DB_INT
  }, 'max-quantity close')

  const maxPriceProductID = await createOnShelfProduct(runID, categoryID, 'max-unit-price', 1)
  const maxPriceOrder = await createOrder(maxPriceProductID, 1, MAX_DB_INT, 'signed INT unit-price maximum')
  assert(
    maxPriceOrder.code === CODE.OK &&
      Number(maxPriceOrder.data.quantity) === 1 &&
      Number(maxPriceOrder.data.deal_price_cent) === MAX_DB_INT &&
      Number(maxPriceOrder.data.total_deal_price_cent) === MAX_DB_INT,
    'signed INT unit-price maximum should produce an exact safe-integer total',
    maxPriceOrder
  )
  const maxPriceClose = await closeOrder(maxPriceOrder.data.order_id, `${runID}-max-price-close`)
  assert(maxPriceClose.code === CODE.OK, 'max-unit-price order close failed', maxPriceClose)
}

async function testConcurrentReservation(runID, categoryID) {
  step('2/5 stock=5 with ten concurrent quantity=1 requests')
  const productID = await createOnShelfProduct(runID, categoryID, 'reserve-race', 5)
  const responses = await runConcurrently(10, (index) => createOrder(productID, 1, 2000 + index, `reserve race ${index}`))
  const successes = responses.filter((response) => response.code === CODE.OK)
  const conflicts = responses.filter((response) => response.code === CODE.CONFLICT)
  assert(successes.length === 5 && conflicts.length === 5, 'concurrent reservation must yield exactly five successes and five conflicts', {
    codes: responses.map((response) => response.code),
    successCount: successes.length,
    conflictCount: conflicts.length
  })
  for (const response of successes) {
    assert(
      Number(response.data.quantity) === 1 &&
        Number(response.data.total_deal_price_cent) === Number(response.data.deal_price_cent) &&
        response.data.product_status === 'ON_SHELF',
      'successful concurrent order has an invalid response contract',
      response
    )
  }
  assertInventory(await productDetail(productID), {
    status: 'ON_SHELF', stock: 5, reservedStock: 5, availableStock: 0
  }, 'concurrent reservation')

  const active = await activeOrdersForProduct(productID)
  const successIDs = successes.map((response) => Number(response.data.order_id)).sort((a, b) => a - b)
  const activeIDs = active.map((order) => Number(order.id)).sort((a, b) => a - b)
  assert(active.length === 5 && JSON.stringify(activeIDs) === JSON.stringify(successIDs), 'active orders do not match successful reservations', {
    successIDs,
    activeIDs
  })

  const closes = await runConcurrently(successes.length, (index) => closeOrder(
    successes[index].data.order_id,
    `${runID}-reserve-close-${index}`
  ))
  assert(closes.every((response) => response.code === CODE.OK), 'closing concurrent reservations failed', {
    codes: closes.map((response) => response.code)
  })
  assertInventory(await productDetail(productID), {
    status: 'ON_SHELF', stock: 5, reservedStock: 0, availableStock: 5
  }, 'concurrent close cleanup')
  assert((await activeOrdersForProduct(productID)).length === 0, 'active reservations remain after close cleanup')
}

async function testDoubleComplete(runID, categoryID) {
  step('3/5 concurrent double-complete is inventory-idempotent')
  const productID = await createOnShelfProduct(runID, categoryID, 'double-complete', 2)
  const created = await createOrder(productID, 1, 3100, 'double complete target')
  assert(created.code === CODE.OK, 'double-complete target creation failed', created)
  const orderID = created.data.order_id

  const responses = await runConcurrently(2, (index) => completeOrder(orderID, `${runID}-double-complete-${index}`))
  const changed = responses.filter((response) => response.code === CODE.OK && response.data.idempotent === false)
  const followers = responses.filter((response) =>
    response.code === CODE.CONFLICT || (response.code === CODE.OK && response.data.idempotent === true)
  )
  assert(
    changed.length === 1 && followers.length === 1,
    'double-complete must perform one state change; the competing request must be idempotent or conflict',
    responses
  )
  assertInventory(await productDetail(productID), {
    status: 'ON_SHELF', stock: 1, reservedStock: 0, availableStock: 1
  }, 'double-complete')

  const repeat = await completeOrder(orderID, `${runID}-complete-repeat`)
  assert(repeat.code === CODE.OK && repeat.data.idempotent === true, 'completed order repeat must be idempotent', repeat)
  const opposite = await closeOrder(orderID, `${runID}-complete-opposite`)
  assert(opposite.code === CODE.INVALID_TRANSITION, 'completed order must reject close', opposite)
  const detail = await orderDetail(orderID)
  assert(detail.status === 'COMPLETED', 'double-complete order final status is incorrect', detail)
}

async function testDoubleClose(runID, categoryID) {
  step('4/5 concurrent double-close is inventory-idempotent')
  const productID = await createOnShelfProduct(runID, categoryID, 'double-close', 2)
  const created = await createOrder(productID, 1, 3200, 'double close target')
  assert(created.code === CODE.OK, 'double-close target creation failed', created)
  const orderID = created.data.order_id

  const responses = await runConcurrently(2, (index) => closeOrder(orderID, `${runID}-double-close-${index}`))
  const changed = responses.filter((response) => response.code === CODE.OK && response.data.idempotent === false)
  const followers = responses.filter((response) =>
    response.code === CODE.CONFLICT || (response.code === CODE.OK && response.data.idempotent === true)
  )
  assert(
    changed.length === 1 && followers.length === 1,
    'double-close must perform one state change; the competing request must be idempotent or conflict',
    responses
  )
  assertInventory(await productDetail(productID), {
    status: 'ON_SHELF', stock: 2, reservedStock: 0, availableStock: 2
  }, 'double-close')

  const repeat = await closeOrder(orderID, `${runID}-close-repeat`)
  assert(repeat.code === CODE.OK && repeat.data.idempotent === true, 'closed order repeat must be idempotent', repeat)
  const opposite = await completeOrder(orderID, `${runID}-close-opposite`)
  assert(opposite.code === CODE.INVALID_TRANSITION, 'closed order must reject complete', opposite)
  const detail = await orderDetail(orderID)
  assert(detail.status === 'CLOSED', 'double-close order final status is incorrect', detail)
}

async function testCompleteCloseRace(runID, categoryID) {
  step('5/5 complete-vs-close race with another active reservation')
  const productID = await createOnShelfProduct(runID, categoryID, 'terminal-race', 3)
  const target = await createOrder(productID, 2, 4100, 'terminal race target')
  const guard = await createOrder(productID, 1, 4200, 'terminal race guard')
  assert(target.code === CODE.OK && guard.code === CODE.OK, 'terminal-race orders were not created', { target, guard })
  assertInventory(await productDetail(productID), {
    status: 'ON_SHELF', stock: 3, reservedStock: 3, availableStock: 0
  }, 'terminal-race setup')

  const [complete, close] = await runConcurrently(2, (index) => {
    if (index === 0) return completeOrder(target.data.order_id, `${runID}-race-complete`)
    return closeOrder(target.data.order_id, `${runID}-race-close`)
  })
  const winners = [complete, close].filter((response) => response.code === CODE.OK)
  const losers = [complete, close].filter((response) =>
    response.code === CODE.INVALID_TRANSITION || response.code === CODE.CONFLICT
  )
  assert(winners.length === 1 && losers.length === 1, 'complete-vs-close must have one winner and one conflict/invalid transition', {
    completeCode: complete.code,
    closeCode: close.code
  })

  const winner = winners[0]
  const completed = winner.data.to_status === 'COMPLETED'
  assert(completed || winner.data.to_status === 'CLOSED', 'race winner has an unexpected terminal status', winner)
  assert(winner.data.idempotent === false, 'race winner must perform the state transition', winner)
  assertInventory(await productDetail(productID), completed
    ? { status: 'ON_SHELF', stock: 1, reservedStock: 1, availableStock: 0 }
    : { status: 'ON_SHELF', stock: 3, reservedStock: 1, availableStock: 2 }, 'complete-vs-close race')

  const targetDetail = await orderDetail(target.data.order_id)
  assert(targetDetail.status === winner.data.to_status, 'target order does not match the race winner', targetDetail)
  const repeatWinner = completed
    ? await completeOrder(target.data.order_id, `${runID}-race-repeat-complete`)
    : await closeOrder(target.data.order_id, `${runID}-race-repeat-close`)
  assert(repeatWinner.code === CODE.OK && repeatWinner.data.idempotent === true, 'race winner repeat must be idempotent', repeatWinner)
  const repeatLoser = completed
    ? await closeOrder(target.data.order_id, `${runID}-race-repeat-close`)
    : await completeOrder(target.data.order_id, `${runID}-race-repeat-complete`)
  assert(repeatLoser.code === CODE.INVALID_TRANSITION, 'opposite terminal action must remain invalid', repeatLoser)

  const closeGuard = await closeOrder(guard.data.order_id, `${runID}-race-guard-close`)
  assert(closeGuard.code === CODE.OK, 'guard reservation close failed', closeGuard)
  assertInventory(await productDetail(productID), completed
    ? { status: 'ON_SHELF', stock: 1, reservedStock: 0, availableStock: 1 }
    : { status: 'ON_SHELF', stock: 3, reservedStock: 0, availableStock: 3 }, 'terminal-race cleanup')
  assert((await activeOrdersForProduct(productID)).length === 0, 'active orders remain after terminal-race cleanup')
}

async function bestEffortCleanup(runID) {
  if (merchantToken) {
    console.log('[MYSQL-ACCEPTANCE][CLEANUP] Closing remaining active test orders and test products where possible')
    let consecutiveEmptyLists = 0
    for (let attempt = 1; attempt <= 10 && consecutiveEmptyLists < 2; attempt += 1) {
      try {
        const list = await req('/merchant/orders?status=CREATED&page=1&page_size=100', {
          headers: authorization(merchantToken)
        })
        if (list.code !== CODE.OK) {
          console.warn(`[MYSQL-ACCEPTANCE][CLEANUP][WARN] active-order list returned code ${list.code}`)
          break
        }

        const items = list.data.items || []
        consecutiveEmptyLists = items.length === 0 ? consecutiveEmptyLists + 1 : 0
        for (const item of items) {
          try {
            await closeOrder(item.id, `${runID}-cleanup-order-${item.id}`, 'acceptance best-effort cleanup')
          } catch (error) {
            console.warn(`[MYSQL-ACCEPTANCE][CLEANUP][WARN] order ${item.id}: ${error instanceof Error ? error.message : String(error)}`)
          }
        }
        if (consecutiveEmptyLists < 2) {
          await new Promise((resolve) => setTimeout(resolve, 500))
        }
      } catch (error) {
        console.warn(`[MYSQL-ACCEPTANCE][CLEANUP][WARN] unable to list active orders: ${error instanceof Error ? error.message : String(error)}`)
        break
      }
    }

    for (const productID of productIDs) {
      try {
        const response = await req(`/merchant/products/${productID}/close`, {
          method: 'POST',
          headers: authorization(merchantToken, { 'Idempotency-Key': `${runID}-cleanup-product-${productID}` }),
          body: {}
        })
        if (response.code !== CODE.OK && response.code !== CODE.INVALID_TRANSITION) {
          console.warn(`[MYSQL-ACCEPTANCE][CLEANUP][WARN] product ${productID} returned code ${response.code}`)
        }
      } catch (error) {
        console.warn(`[MYSQL-ACCEPTANCE][CLEANUP][WARN] product ${productID}: ${error instanceof Error ? error.message : String(error)}`)
      }
    }
  }

  for (const token of [merchantToken, adminToken]) {
    if (!token) continue
    try {
      await req('/auth/logout', {
        method: 'POST',
        headers: authorization(token),
        body: {}
      })
    } catch {
      // Session cleanup is best effort; never print tokens or credential material.
    }
  }
  console.log('[MYSQL-ACCEPTANCE][CLEANUP] Database rows remain; recreate the isolated DB/volume for full cleanup')
}

async function main() {
  configureTarget()
  const runID = uniqueRunID()
  const adminUsername = requiredEnv('SMOKE_ADMIN_USERNAME')
  const adminPassword = requiredEnv('SMOKE_ADMIN_PASSWORD')

  console.log(`[MYSQL-ACCEPTANCE] run_id=${runID}`)
  console.log('[MYSQL-ACCEPTANCE] This run mutates an isolated database and does not delete its rows')
  try {
    await healthCheck()
    const { categoryID, merchantID, username } = await setupMerchant(runID, adminUsername, adminPassword)
    console.log(`[MYSQL-ACCEPTANCE] test_merchant_id=${merchantID} test_username=${username}`)

    await testInputBoundaries(runID, categoryID)
    await testConcurrentReservation(runID, categoryID)
    await testDoubleComplete(runID, categoryID)
    await testDoubleClose(runID, categoryID)
    await testCompleteCloseRace(runID, categoryID)

    console.log('[MYSQL-ACCEPTANCE][PASS] MySQL multi-stock reservation and terminal-race API checks passed')
  } finally {
    await bestEffortCleanup(runID)
  }
}

main().catch((error) => {
  console.error(`[MYSQL-ACCEPTANCE][ERROR] ${error instanceof Error ? error.message : String(error)}`)
  process.exitCode = 1
})
