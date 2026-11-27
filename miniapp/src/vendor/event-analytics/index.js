'use strict'

const maxQueueSize = 200
const batchSize = 50
const maxAge = 7 * 24 * 60 * 60 * 1000
const flushDelay = 1500
const identifier = /^[a-z][a-z0-9]*(_[a-z0-9]+)*$/
const route = /^[A-Za-z0-9_-]+(\/[A-Za-z0-9_-]+)+$/
const sensitiveKey = /(?:^|_)(?:token|password|passwd|secret|credential|authorization|cookie|phone|mobile|email|openid|unionid|address|idcard|session|id_card|open_id|union_id|access_key|api_key)(?:_|$)/
const trackers = new WeakMap()
let sequence = 0

function makeId(prefix) {
  return prefix + '_' + Date.now().toString(36) + '_' + Math.random().toString(36).slice(2, 12) + '_' + (++sequence).toString(36)
}

function validIdentifier(value, limit) {
  return typeof value === 'string' && value.length <= limit && identifier.test(value)
}

function validRoute(value) {
  return typeof value === 'string' && value.length <= 128 && route.test(value)
}

function propertiesSnapshot(input) {
  if (!input || typeof input !== 'object' || Array.isArray(input)) return
  const prototype = Object.getPrototypeOf(input)
  if (prototype !== null && Object.getPrototypeOf(prototype) !== null) return
  const keys = Object.keys(input)
  if (keys.length > 16) return
  const result = {}
  for (const key of keys) {
    if (!validIdentifier(key, 48) || sensitiveKey.test(key)) return
    const value = input[key]
    switch (typeof value) {
      case 'number':
        if (!Number.isFinite(value) || Math.abs(value) > Number.MAX_SAFE_INTEGER) return
        break
      case 'string':
        if (value.length > 256 || Array.from(value).length > 128 || /[\u0000-\u001f\u007f-\u009f]/.test(value)) return
        break
      case 'boolean':
        break
      default:
        return
    }
    result[key] = value
  }
  return result
}

function fields(name, page, source, properties) {
  if (!validIdentifier(name, 64)) return
  if (page !== undefined && page !== '' && !validRoute(page)) return
  if (name === 'page_view' && !page) return
  if (source !== undefined && source !== '' && !validIdentifier(source, 64)) return
  const event = { name }
  if (page) event.page = page
  if (source) event.source = source
  if (properties !== undefined) {
    const snapshot = propertiesSnapshot(properties)
    if (!snapshot) return
    event.properties = snapshot
  }
  return event
}

function currentRoute(runtime) {
  try {
    const instance = runtime.getCurrentInstance && runtime.getCurrentInstance()
    let path = instance && instance.router && instance.router.path
    if (!path) {
      const pages = runtime.getCurrentPages ? runtime.getCurrentPages()
        : typeof getCurrentPages === 'function' ? getCurrentPages() : []
      const page = pages[pages.length - 1]
      path = page && page.route
    }
    if (typeof path === 'string') {
      path = path.split(/[?#]/, 1)[0].replace(/^\//, '')
      if (validRoute(path)) return path
    }
  } catch {
    // Launch and background events may occur without an active page.
  }
}

function validTime(value, now) {
  if (typeof value !== 'string') return false
  const time = Date.parse(value)
  return time >= now - maxAge && time <= now + 600000
}

function read(runtime, key) {
  try { return runtime.getStorageSync(key) } catch { return undefined }
}

function write(runtime, key, value) {
  try { runtime.setStorageSync(key, value) } catch { /* The in-memory queue still works. */ }
}

function restoredEvents(records) {
  const restored = []
  if (!Array.isArray(records)) return restored
  const ids = new Set()
  const now = Date.now()
  for (const record of records.slice(-maxQueueSize)) {
    if (!record || typeof record.event_id !== 'string' || !/^[A-Za-z0-9_-]{1,64}$/.test(record.event_id) ||
      ids.has(record.event_id) || !validTime(record.occurred_at, now)) continue
    const event = fields(record.name, record.page, record.source, record.properties)
    if (!event) continue
    ids.add(record.event_id)
    restored.push({ ...event, event_id: record.event_id, occurred_at: record.occurred_at })
  }
  return restored
}

function request(runtime, options) {
  return new Promise((resolve, reject) => {
    const result = runtime.request({ ...options, success: resolve, fail: reject })
    // Native APIs return a request task. Taro also supports promise-based adapters.
    if (result && typeof result.then === 'function') result.then(resolve, reject)
  })
}

function createTracker({ endpoint, projectKey, platform, runtime }) {
  if (typeof endpoint !== 'string' || !/^https?:\/\/[^\s?#]+$/.test(endpoint) ||
    !projectKey || typeof projectKey !== 'string' || /\s/.test(projectKey) ||
    (platform !== 'weapp' && platform !== 'tt') || !runtime || typeof runtime.request !== 'function') {
    throw new TypeError('Expected an event endpoint, project write key, platform and miniapp runtime')
  }
  const scope = 'event_analytics_v1:' + platform + ':' + encodeURIComponent(endpoint) + ':' + encodeURIComponent(projectKey)
  let projects = trackers.get(runtime)
  if (!projects) {
    projects = new Map()
    trackers.set(runtime, projects)
  }
  if (projects.has(scope)) return projects.get(scope)

  const deviceKey = scope + ':device'
  let deviceId = read(runtime, deviceKey)
  if (typeof deviceId !== 'string' || !/^[A-Za-z0-9_-]{1,64}$/.test(deviceId)) {
    deviceId = makeId('d')
    write(runtime, deviceKey, deviceId)
  }
  const queueKey = scope + ':queue'
  const saved = read(runtime, queueKey)
  let queue = restoredEvents(saved)
  if (Array.isArray(saved) && saved.length) write(runtime, queueKey, queue)
  let timer
  let sending
  let paused = false
  let retryDelay = 0

  function schedule() {
    if (!paused && !sending && !timer && queue.length) {
      timer = setTimeout(flush, retryDelay || flushDelay)
    }
  }

  async function send() {
    // Take a snapshot so new events can be queued while the network is busy.
    const now = Date.now()
    const recent = queue.filter(event => validTime(event.occurred_at, now))
    if (recent.length !== queue.length) {
      queue = recent
      write(runtime, queueKey, queue)
    }
    for (let offset = 0; offset < recent.length; offset += batchSize) {
      const events = recent.slice(offset, offset + batchSize)
      const response = await request(runtime, {
        url: endpoint, method: 'POST', timeout: 10000,
        header: { 'Content-Type': 'application/json', 'X-Project-Key': projectKey },
        data: { platform, device_id: deviceId, events }
      })
      const receipt = response && response.data && response.data.data
      if (!response || !Number.isInteger(response.statusCode) || response.statusCode < 200 || response.statusCode >= 300 ||
        !receipt || receipt.received !== events.length || !Number.isInteger(receipt.discarded) ||
        receipt.discarded < 0 || receipt.discarded > events.length) {
        throw new Error('Event batch was not acknowledged')
      }
      const done = new Set(events.map(event => event.event_id))
      queue = queue.filter(event => !done.has(event.event_id))
      write(runtime, queueKey, queue)
    }
  }

  function flush() {
    clearTimeout(timer)
    timer = undefined
    if (sending) return sending
    if (!queue.length) return Promise.resolve(true)
    sending = send().then(() => {
      retryDelay = 0
      return true
    }, () => {
      retryDelay = Math.min((retryDelay || flushDelay) * 2, 60000)
      return false
    }).then(ok => {
      sending = undefined
      schedule()
      return ok
    })
    return sending
  }

  const tracker = {
    track(name, options = {}) {
      if (!options || typeof options !== 'object' || Array.isArray(options)) return false
      const page = options.page === undefined ? currentRoute(runtime) : options.page
      const event = fields(name, page, options.source, options.properties)
      if (!event) return false
      queue.push({ ...event, event_id: makeId('e'), occurred_at: new Date().toISOString() })
      if (queue.length > maxQueueSize) queue.shift()
      write(runtime, queueKey, queue)
      schedule()
      return true
    },
    flush,
    pause() { paused = true; void flush() },
    resume() { paused = false; schedule() }
  }
  projects.set(scope, tracker)
  schedule()
  return tracker
}

exports.createTracker = createTracker
