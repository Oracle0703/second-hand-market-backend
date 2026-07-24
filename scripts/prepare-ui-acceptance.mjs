#!/usr/bin/env node

/**
 * Create and approve a disposable merchant for isolated browser acceptance.
 * Credentials are written once to a caller-provided temporary file (0600).
 */

import { randomBytes } from 'node:crypto'
import { writeFile } from 'node:fs/promises'
import path from 'node:path'

const ISOLATED_CONFIRMATION = 'I_UNDERSTAND_THIS_CREATES_AN_ACCEPTANCE_MERCHANT'
const LOOPBACK_HOSTS = new Set(['127.0.0.1', 'localhost', '[::1]'])
const REQUEST_TIMEOUT_MS = 20_000

function requiredEnv(name) {
  const value = String(process.env[name] || '').trim()
  if (!value) throw new Error(`${name} is required`)
  return value
}

function configureTarget() {
  const rawBaseURL = requiredEnv('API_BASE_URL')
  const parsed = new URL(rawBaseURL)
  if (!LOOPBACK_HOSTS.has(parsed.hostname) || !/\/api\/v1\/?$/.test(parsed.pathname)) {
    throw new Error('API_BASE_URL must be a loopback URL ending in /api/v1')
  }
  if (requiredEnv('ACCEPTANCE_CONFIRM_ISOLATED') !== ISOLATED_CONFIRMATION) {
    throw new Error(`ACCEPTANCE_CONFIRM_ISOLATED must equal ${ISOLATED_CONFIRMATION}`)
  }
  return rawBaseURL.replace(/\/$/, '')
}

async function req(baseURL, requestPath, { method = 'GET', headers = {}, body } = {}) {
  const response = await fetch(baseURL + requestPath, {
    method,
    headers: { 'Content-Type': 'application/json', ...headers },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS)
  })
  try {
    return await response.json()
  } catch {
    throw new Error(`non-JSON response for ${method} ${requestPath} (HTTP ${response.status})`)
  }
}

function assert(condition, message, payload) {
  if (condition) return
  const code = payload && typeof payload === 'object' ? ` (code=${payload.code})` : ''
  throw new Error(`${message}${code}`)
}

async function main() {
  const baseURL = configureTarget()
  const adminUsername = requiredEnv('SMOKE_ADMIN_USERNAME')
  const adminPassword = requiredEnv('SMOKE_ADMIN_PASSWORD')
  const credentialsFile = requiredEnv('UI_ACCEPTANCE_CREDENTIALS_FILE')
  if (!path.isAbsolute(credentialsFile)) {
    throw new Error('UI_ACCEPTANCE_CREDENTIALS_FILE must be an absolute temporary path')
  }

  const runID = `${Date.now().toString(36)}_${randomBytes(5).toString('hex')}`
  const username = `ui_accept_${runID}`
  const password = `Ui!${randomBytes(24).toString('base64url')}`
  const phoneSuffix = String(randomBytes(4).readUInt32BE() % 100_000_000).padStart(8, '0')

  const license = await req(baseURL, '/files/presign', {
    method: 'POST',
    body: {
      biz_type: 'MERCHANT_LICENSE',
      file_name: `ui-accept-license-${runID}.jpg`,
      file_size: 1024,
      mime_type: 'image/jpeg'
    }
  })
  assert(license.code === 0, 'license presign failed', license)

  const register = await req(baseURL, '/auth/register', {
    method: 'POST',
    body: {
      merchant_name: `UI Acceptance ${runID}`,
      contact_name: 'Acceptance Operator',
      phone: `137${phoneSuffix}`,
      username,
      password,
      license_file_id: license.data.file_id
    }
  })
  assert(register.code === 0, 'merchant registration failed', register)

  const adminLogin = await req(baseURL, '/auth/login', {
    method: 'POST',
    body: { login_type: 'ADMIN', username: adminUsername, password: adminPassword }
  })
  assert(adminLogin.code === 0, 'acceptance administrator login failed', adminLogin)

  const approve = await req(baseURL, `/admin/merchants/${register.data.merchant_id}/approve`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${adminLogin.data.access_token}` },
    body: { comment: `isolated UI acceptance ${runID}` }
  })
  assert(approve.code === 0, 'UI acceptance merchant approval failed', approve)

  const merchantLogin = await req(baseURL, '/auth/login', {
    method: 'POST',
    body: { login_type: 'MERCHANT', username, password }
  })
  assert(merchantLogin.code === 0 && merchantLogin.data.token_scope === 'full', 'approved merchant login failed', merchantLogin)
  const merchantHeaders = { Authorization: `Bearer ${merchantLogin.data.access_token}` }

  const categories = await req(baseURL, '/merchant/categories?level=2', { headers: merchantHeaders })
  assert(categories.code === 0 && categories.data.items?.length > 0, 'no level-2 category is available', categories)
  const categoryID = categories.data.items[0].id || categories.data.items[0].ID

  const image = await req(baseURL, '/files/presign', {
    method: 'POST',
    headers: merchantHeaders,
    body: {
      biz_type: 'PRODUCT_IMAGE',
      file_name: `ui-accept-product-${runID}.jpg`,
      file_size: 1024,
      mime_type: 'image/jpeg'
    }
  })
  assert(image.code === 0, 'product image presign failed', image)

  const product = await req(baseURL, '/merchant/products', {
    method: 'POST',
    headers: merchantHeaders,
    body: {
      title: `UI acceptance cups ${runID}`,
      description: 'Disposable product for isolated browser acceptance',
      category_id: categoryID,
      price_cent: 6800,
      original_price_cent: 8800,
      condition_level: 'LIKE_NEW',
      stock: 5,
      image_file_ids: [image.data.file_id]
    }
  })
  assert(product.code === 0, 'UI acceptance product creation failed', product)

  const onShelf = await req(baseURL, `/merchant/products/${product.data.product_id}/on-shelf`, {
    method: 'POST',
    headers: merchantHeaders,
    body: {}
  })
  assert(onShelf.code === 0, 'UI acceptance product on-shelf failed', onShelf)

  await writeFile(credentialsFile, `${JSON.stringify({
    username,
    password,
    merchant_id: register.data.merchant_id,
    product_id: product.data.product_id
  })}\n`, { encoding: 'utf8', mode: 0o600, flag: 'wx' })

  console.log(`[UI-ACCEPTANCE][PASS] approved disposable merchant_id=${register.data.merchant_id} username=${username}`)
  console.log('[UI-ACCEPTANCE] credentials written to the requested 0600 temporary file')
}

main().catch((error) => {
  console.error(`[UI-ACCEPTANCE][ERROR] ${error instanceof Error ? error.message : String(error)}`)
  process.exitCode = 1
})
