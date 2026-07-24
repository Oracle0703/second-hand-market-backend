#!/usr/bin/env node

/**
 * Administrator password/session acceptance for an isolated environment.
 *
 * This changes the target acceptance administrator to a random password and
 * intentionally does not print or persist the new value. Keep a separate
 * control administrator for later smoke tests. Never target production.
 */

import { randomBytes } from 'node:crypto'

const CODE = {
  OK: 0,
  INVALID_ARGUMENT: 10001,
  UNAUTHORIZED: 10002
}

const ISOLATED_CONFIRMATION = 'I_UNDERSTAND_THIS_CHANGES_AN_ACCEPTANCE_PASSWORD'
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

function redact(value) {
  if (Array.isArray(value)) return value.map(redact)
  if (!value || typeof value !== 'object') return value
  return Object.fromEntries(Object.entries(value).map(([key, item]) => {
    const normalized = key.toLowerCase()
    if (normalized.includes('password') || normalized.includes('token') || normalized.includes('secret')) {
      return [key, '[REDACTED]']
    }
    return [key, redact(item)]
  }))
}

function assert(condition, message, payload) {
  if (condition) return
  const detail = payload === undefined ? '' : `\n${JSON.stringify(redact(payload), null, 2)}`
  throw new Error(`${message}${detail}`)
}

async function req(baseURL, path, { method = 'GET', headers = {}, body } = {}) {
  const response = await fetch(baseURL + path, {
    method,
    headers: { 'Content-Type': 'application/json', ...headers },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS)
  })
  try {
    return await response.json()
  } catch {
    throw new Error(`non-JSON response for ${method} ${path} (HTTP ${response.status})`)
  }
}

function auth(token) {
  return { Authorization: `Bearer ${token}` }
}

async function login(baseURL, username, password) {
  return req(baseURL, '/auth/login', {
    method: 'POST',
    body: { login_type: 'ADMIN', username, password }
  })
}

async function main() {
  const baseURL = configureTarget()
  const targetUsername = requiredEnv('SMOKE_TARGET_ADMIN_USERNAME')
  const targetPassword = requiredEnv('SMOKE_TARGET_ADMIN_PASSWORD')
  const controlUsername = requiredEnv('SMOKE_CONTROL_ADMIN_USERNAME')
  const controlPassword = requiredEnv('SMOKE_CONTROL_ADMIN_PASSWORD')
  assert(targetUsername !== controlUsername, 'target and control administrators must be different accounts')

  console.log('[ADMIN-ACCEPTANCE][STEP] Login target and control administrators')
  const targetLogin = await login(baseURL, targetUsername, targetPassword)
  const controlLogin = await login(baseURL, controlUsername, controlPassword)
  assert(targetLogin.code === CODE.OK, 'target administrator login failed', targetLogin)
  assert(controlLogin.code === CODE.OK, 'control administrator login failed', controlLogin)

  const oldAccess = targetLogin.data.access_token
  const oldRefresh = targetLogin.data.refresh_token
  const controlAccess = controlLogin.data.access_token

  console.log('[ADMIN-ACCEPTANCE][STEP] Reject invalid password changes')
  const wrongCurrent = await req(baseURL, '/admin/account/password', {
    method: 'PUT',
    headers: auth(oldAccess),
    body: { current_password: 'definitely-not-the-current-password', new_password: `Adm!${randomBytes(20).toString('base64url')}` }
  })
  assert(wrongCurrent.code === CODE.INVALID_ARGUMENT, 'wrong current password must be rejected', wrongCurrent)

  const weakPassword = await req(baseURL, '/admin/account/password', {
    method: 'PUT',
    headers: auth(oldAccess),
    body: { current_password: targetPassword, new_password: 'short' }
  })
  assert(weakPassword.code === CODE.INVALID_ARGUMENT, 'weak new password must be rejected', weakPassword)

  console.log('[ADMIN-ACCEPTANCE][STEP] Change only the target administrator password')
  const newPassword = `Adm!${randomBytes(32).toString('base64url')}`
  const changed = await req(baseURL, '/admin/account/password', {
    method: 'PUT',
    headers: auth(oldAccess),
    body: { current_password: targetPassword, new_password: newPassword }
  })
  assert(changed.code === CODE.OK, 'target administrator password change failed', changed)

  console.log('[ADMIN-ACCEPTANCE][STEP] Prove old credentials/session are revoked')
  const oldAccessResult = await req(baseURL, '/admin/logs?page=1&page_size=1', {
    headers: auth(oldAccess)
  })
  assert(oldAccessResult.code === CODE.UNAUTHORIZED, 'old access token must fail immediately', oldAccessResult)

  const oldRefreshResult = await req(baseURL, '/auth/refresh', {
    method: 'POST',
    body: { refresh_token: oldRefresh }
  })
  assert(oldRefreshResult.code === CODE.UNAUTHORIZED, 'old refresh token must fail immediately', oldRefreshResult)

  const oldPasswordResult = await login(baseURL, targetUsername, targetPassword)
  assert(oldPasswordResult.code === CODE.UNAUTHORIZED, 'old password must fail after change', oldPasswordResult)

  const newPasswordResult = await login(baseURL, targetUsername, newPassword)
  assert(newPasswordResult.code === CODE.OK, 'new password login failed', newPasswordResult)

  console.log('[ADMIN-ACCEPTANCE][STEP] Prove the control administrator remains active')
  const controlResult = await req(baseURL, '/admin/logs?page=1&page_size=1', {
    headers: auth(controlAccess)
  })
  assert(controlResult.code === CODE.OK, 'control administrator session was revoked', controlResult)

  for (const token of [newPasswordResult.data.access_token, controlAccess]) {
    try {
      await req(baseURL, '/auth/logout', { method: 'POST', headers: auth(token), body: {} })
    } catch {
      // Logout is best effort and credential material is never logged.
    }
  }

  console.log('[ADMIN-ACCEPTANCE][PASS] Target-only password rotation and immediate session revocation passed')
  console.log('[ADMIN-ACCEPTANCE] Target password is intentionally disposable; use the unchanged control administrator for later tests')
}

main().catch((error) => {
  console.error(`[ADMIN-ACCEPTANCE][ERROR] ${error instanceof Error ? error.message : String(error)}`)
  process.exitCode = 1
})
