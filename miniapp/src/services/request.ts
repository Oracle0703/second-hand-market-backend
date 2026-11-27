import Taro from '@tarojs/taro'
import { ensureDeviceID, useSessionStore } from '../stores/session'

declare const __API_BASE_URL__: string
declare const __DEV_MODE__: boolean

type APIResponse<T> = {
  code: number
  message: string
  request_id: string
  data: T
}

type RequestOptions<T> = {
  method: 'GET' | 'POST' | 'DELETE'
  path: string
  data?: Record<string, unknown>
  skipAuth?: boolean
  retrying?: boolean
  expectedSession?: SessionIdentity
}

type UnknownRecord = Record<string, unknown>
type SessionIdentity = {
  accessToken: string
  refreshToken: string
}
type RefreshOutcome =
  | { status: 'refreshed'; owner: SessionIdentity; session: SessionIdentity }
  | { status: 'failed'; owner: SessionIdentity }
  | { status: 'stale'; owner: SessionIdentity }
  | { status: 'transient'; owner: SessionIdentity; error: unknown }
type RefreshFlight = {
  owner: SessionIdentity
  promise: Promise<RefreshOutcome>
  outcome?: RefreshOutcome
}

const REFRESH_AUTH_FAILURE_CODES = new Set([10002, 10003, 10007])
const BASE_URL = (typeof __API_BASE_URL__ === 'string' && __API_BASE_URL__.trim()) || 'https://market.meaningful.ink/api/v1'
let refreshFlight: RefreshFlight | null = null

export class AuthExpiredError extends Error {
  readonly code = 10002

  constructor() {
    super('登录已过期，请重新登录')
    this.name = 'AuthExpiredError'
  }
}

function buildURL(path: string): string {
  if (path.startsWith('http')) return path
  return `${BASE_URL}${path}`
}

function withQuery(path: string, data?: Record<string, unknown>): string {
  if (!data || Object.keys(data).length === 0) return path
  const params = Object.entries(data)
    .filter(([, value]) => value !== undefined && value !== null && value !== '')
    .map(([key, value]) => `${encodeURIComponent(key)}=${encodeURIComponent(String(value))}`)
    .join('&')
  if (!params) return path
  return `${path}${path.includes('?') ? '&' : '?'}${params}`
}

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === 'object' && value !== null
}

function isAPIResponse<T>(payload: unknown): payload is APIResponse<T> {
  return isRecord(payload) && typeof payload.code === 'number'
}

function isUnauthorized(statusCode: number, payload: unknown): boolean {
  return statusCode === 401 || (isAPIResponse(payload) && payload.code === 10002)
}

function hasRefreshTokens(value: unknown): value is { access_token: string; refresh_token: string } {
  return isRecord(value) &&
    typeof value.access_token === 'string' && value.access_token.trim() !== '' &&
    typeof value.refresh_token === 'string' && value.refresh_token.trim() !== ''
}

function sessionIdentity(): SessionIdentity {
  const state = useSessionStore.getState()
  return {
    accessToken: state.accessToken,
    refreshToken: state.refreshToken
  }
}

function isSameSession(left: SessionIdentity, right: SessionIdentity): boolean {
  return left.accessToken === right.accessToken && left.refreshToken === right.refreshToken
}

function clearSessionIfOwnedBy(owner: SessionIdentity): void {
  const current = useSessionStore.getState()
  if (isSameSession(current, owner)) {
    current.clearSession()
  }
}

function failOwnedRefresh(owner: SessionIdentity): RefreshOutcome {
  if (!isSameSession(sessionIdentity(), owner)) {
    return { status: 'stale', owner }
  }
  useSessionStore.getState().clearSession()
  return { status: 'failed', owner }
}

function transientRefresh(owner: SessionIdentity, error: unknown): RefreshOutcome {
  if (!isSameSession(sessionIdentity(), owner)) {
    return { status: 'stale', owner }
  }
  return { status: 'transient', owner, error }
}

function isRefreshAuthFailure(statusCode: number, payload: unknown): boolean {
  return statusCode === 401 || statusCode === 403 ||
    (isAPIResponse(payload) && REFRESH_AUTH_FAILURE_CODES.has(payload.code))
}

function formatPayloadSummary(payload: unknown): string {
  if (typeof payload === 'string') {
    return `string:${payload.slice(0, 200)}`
  }
  if (isRecord(payload)) {
    return `object keys=${Object.keys(payload).slice(0, 10).join(',') || '(none)'}`
  }
  if (payload === null) {
    return 'null'
  }
  return typeof payload
}

function buildMalformedResponseError(res: Taro.request.SuccessCallbackResult<unknown>, url: string): Error {
  const detail = `API response malformed: status=${res.statusCode}, url=${url}, payload=${formatPayloadSummary(res.data)}`
  return new Error(detail)
}

async function refreshAccessToken(owner: SessionIdentity): Promise<RefreshOutcome> {
  const url = buildURL('/buyer/auth/refresh')
  let res
  try {
    res = await Taro.request<APIResponse<{ access_token: string; refresh_token: string }>>({
      url,
      method: 'POST',
      data: { refresh_token: owner.refreshToken },
      header: {
        'Content-Type': 'application/json',
        'X-Device-Id': ensureDeviceID()
      }
    })
  } catch (err) {
    if (__DEV_MODE__) {
      console.error('[miniapp-api] refresh request failed', url, err)
    }
    return transientRefresh(owner, err)
  }

  const payload = res.data
  // Only an explicit auth rejection proves the refresh token is unusable;
  // transport errors, 5xx and other unexpected responses keep the session.
  if (isRefreshAuthFailure(res.statusCode, payload)) {
    return failOwnedRefresh(owner)
  }
  if (res.statusCode < 200 || res.statusCode >= 300) {
    if (__DEV_MODE__) {
      console.error('[miniapp-api] refresh failed response', res.statusCode, res.header, res.data)
    }
    return transientRefresh(
      owner,
      new Error(`request failed: status=${res.statusCode}, url=${url}, payload=${formatPayloadSummary(payload)}`)
    )
  }
  if (!isAPIResponse<{ access_token: string; refresh_token: string }>(payload)) {
    if (__DEV_MODE__) {
      console.error('[miniapp-api] refresh malformed response', res.statusCode, res.header, res.data)
    }
    return transientRefresh(owner, buildMalformedResponseError(res as Taro.request.SuccessCallbackResult<unknown>, url))
  }
  if (payload.code !== 0 || !hasRefreshTokens(payload.data)) {
    return transientRefresh(owner, new Error(payload.message || `request failed: ${payload.code}`))
  }

  const current = useSessionStore.getState()
  if (!isSameSession(current, owner)) {
    return { status: 'stale', owner }
  }

  const refreshedSession = {
    accessToken: payload.data.access_token,
    refreshToken: payload.data.refresh_token
  }
  try {
    current.setSession(refreshedSession.accessToken, refreshedSession.refreshToken, current.profile)
  } catch {
    // The server has already rotated the refresh token, so the old one is unusable.
    return failOwnedRefresh(owner)
  }

  // Zustand subscribers run synchronously. Re-check after setSession so a
  // concurrent login/logout cannot be overwritten or used for the replay.
  if (!isSameSession(sessionIdentity(), refreshedSession)) {
    return { status: 'stale', owner }
  }
  return { status: 'refreshed', owner, session: refreshedSession }
}

function refreshFor(owner: SessionIdentity): Promise<RefreshOutcome> {
  const existing = refreshFlight
  if (existing && isSameSession(existing.owner, owner)) {
    const current = sessionIdentity()
    // A transient outcome keeps the session, so the next 401 starts a new flight.
    if (!existing.outcome ||
      existing.outcome.status === 'refreshed' ||
      !isSameSession(current, owner)) {
      return existing.promise
    }
  }
  if (existing && !existing.outcome && !isSameSession(existing.owner, owner)) {
    return existing.promise.then(() => refreshFor(owner))
  }

  if (!isSameSession(sessionIdentity(), owner)) {
    return Promise.resolve({ status: 'stale', owner })
  }

  let flight!: RefreshFlight
  const promise = refreshAccessToken(owner)
    .catch((err: unknown) => transientRefresh(owner, err))
    .then((outcome) => {
      flight.outcome = outcome
      return outcome
    })
  flight = { owner, promise }
  refreshFlight = flight
  return promise
}

export async function apiRequest<T>(options: RequestOptions<T>): Promise<T> {
  const deviceID = ensureDeviceID()
  const state = useSessionStore.getState()
  const requestSession = {
    accessToken: state.accessToken,
    refreshToken: state.refreshToken
  }
  if (options.expectedSession && !isSameSession(requestSession, options.expectedSession)) {
    throw new AuthExpiredError()
  }
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    'X-Device-Id': deviceID
  }
  if (!options.skipAuth && state.accessToken) {
    headers.Authorization = `Bearer ${state.accessToken}`
  }

  const path = options.method === 'GET' ? withQuery(options.path, options.data) : options.path
  const url = buildURL(path)
  if (__DEV_MODE__) {
    console.log('[miniapp-api] request', options.method, url)
  }
  let res
  try {
    res = await Taro.request<APIResponse<T>>({
      url,
      method: options.method,
      data: options.method === 'GET' ? undefined : options.data,
      header: headers
    })
  } catch (err) {
    if (__DEV_MODE__) {
      console.error('[miniapp-api] request failed', options.method, url, err)
    }
    throw err
  }
  const payload = res.data

  if (__DEV_MODE__) {
    console.log('[miniapp-api] response', options.method, url, res.statusCode, res.header, payload)
  }

  if (isUnauthorized(res.statusCode, payload) && !options.skipAuth) {
    if (options.retrying || !requestSession.refreshToken) {
      clearSessionIfOwnedBy(requestSession)
      throw new AuthExpiredError()
    }

    const outcome = await refreshFor(requestSession)
    if (outcome.status === 'refreshed' &&
      isSameSession(outcome.owner, requestSession) &&
      isSameSession(sessionIdentity(), outcome.session)) {
      return apiRequest<T>({
        ...options,
        retrying: true,
        expectedSession: outcome.session
      })
    }
    if (outcome.status === 'transient') {
      throw outcome.error
    }
    throw new AuthExpiredError()
  }

  if (res.statusCode < 200 || res.statusCode >= 300) {
    throw new Error(`request failed: status=${res.statusCode}, url=${url}, payload=${formatPayloadSummary(payload)}`)
  }

  if (!isAPIResponse<T>(payload)) {
    throw buildMalformedResponseError(res as Taro.request.SuccessCallbackResult<unknown>, url)
  }

  if (payload.code === 0) {
    return payload.data
  }

  throw new Error(payload.message || `request failed: ${payload.code}`)
}
