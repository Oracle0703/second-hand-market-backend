export type Platform = 'weapp' | 'tt'
export type Properties = Record<string, string | number | boolean>

export interface TrackOptions {
  page?: string
  source?: string
  properties?: Properties
}

export interface MiniappRuntime {
  getStorageSync(key: string): unknown
  setStorageSync(key: string, value: any): void
  request(options: {
    url: string
    method: 'POST'
    header: Record<string, string>
    data: Record<string, unknown>
    timeout: number
    success: (response: { statusCode: number; data: unknown }) => void
    fail: (error: unknown) => void
  }): unknown
  getCurrentInstance?: () => { router?: { path?: string } | null }
  getCurrentPages?: () => Array<{ route?: string }>
}

export interface Tracker {
  /** True when a valid event has been queued locally. */
  track(name: string, options?: TrackOptions): boolean
  /** Sends the pending batches. Returns false if delivery must be retried. */
  flush(): Promise<boolean>
  pause(): void
  resume(): void
}

export function createTracker(config: {
  endpoint: string
  projectKey: string
  platform: Platform
  runtime: MiniappRuntime
}): Tracker
