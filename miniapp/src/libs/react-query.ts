import React, { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'

type QueryKey = readonly unknown[]
type QueryStatus = 'pending' | 'success' | 'error'
type FetchStatus = 'idle' | 'fetching'
type MutationStatus = 'idle' | 'pending' | 'success' | 'error'

type QueryOptions<TData> = {
  queryKey: QueryKey
  queryFn: () => Promise<TData> | TData
  enabled?: boolean
  retry?: boolean | number
}

type InvalidateQueryFilters = {
  queryKey?: QueryKey
}

type MutationOptions<TData, TVariables> = {
  mutationFn: (variables: TVariables) => Promise<TData> | TData
  onSuccess?: (data: TData, variables: TVariables) => void | Promise<void>
  onError?: (error: unknown, variables: TVariables) => void | Promise<void>
  onSettled?: (data: TData | undefined, error: unknown, variables: TVariables) => void | Promise<void>
}

type QueryResult<TData> = {
  data: TData | undefined
  error: unknown
  status: QueryStatus
  fetchStatus: FetchStatus
  isLoading: boolean
  refetch: () => Promise<TData | undefined>
}

type MutationResult<TData, TVariables> = {
  data: TData | undefined
  error: unknown
  status: MutationStatus
  isPending: boolean
  mutate: (variables: TVariables) => void
  mutateAsync: (variables: TVariables) => Promise<TData>
  reset: () => void
}

type QueryRegistration = {
  queryKey: QueryKey
  refetch: () => Promise<unknown>
}

function stableSerialize(value: unknown): string {
  if (Array.isArray(value)) {
    return `[${value.map((item) => stableSerialize(item)).join(',')}]`
  }

  if (value && typeof value === 'object') {
    return `{${Object.keys(value)
      .sort()
      .map((key) => `${JSON.stringify(key)}:${stableSerialize((value as Record<string, unknown>)[key])}`)
      .join(',')}}`
  }

  return JSON.stringify(value)
}

function matchesQueryKey(current: QueryKey, target?: QueryKey): boolean {
  if (!target || target.length === 0) {
    return true
  }

  if (target.length > current.length) {
    return false
  }

  return target.every((item, index) => stableSerialize(item) === stableSerialize(current[index]))
}

async function runWithRetry<T>(fn: () => Promise<T> | T, retry?: boolean | number): Promise<T> {
  const maxRetries = retry === false ? 0 : typeof retry === 'number' ? retry : 0
  let attempt = 0

  while (true) {
    try {
      return await Promise.resolve(fn())
    } catch (error) {
      if (attempt >= maxRetries) {
        throw error
      }

      attempt += 1
    }
  }
}

export class QueryClient {
  private readonly registrations = new Map<string, Set<QueryRegistration>>()

  registerQuery(queryKey: QueryKey, refetch: () => Promise<unknown>) {
    const hash = stableSerialize(queryKey)
    const existing = this.registrations.get(hash) || new Set<QueryRegistration>()
    const registration = { queryKey, refetch }

    existing.add(registration)
    this.registrations.set(hash, existing)

    return () => {
      const group = this.registrations.get(hash)
      if (!group) {
        return
      }

      group.delete(registration)
      if (group.size === 0) {
        this.registrations.delete(hash)
      }
    }
  }

  async invalidateQueries(filters: InvalidateQueryFilters = {}): Promise<void> {
    const tasks: Promise<unknown>[] = []

    this.registrations.forEach((group) => {
      group.forEach((registration) => {
        if (matchesQueryKey(registration.queryKey, filters.queryKey)) {
          tasks.push(registration.refetch())
        }
      })
    })

    await Promise.allSettled(tasks)
  }
}

const QueryClientContext = createContext<QueryClient | null>(null)

export function QueryClientProvider(props: { client: QueryClient; children: React.ReactNode }) {
  return React.createElement(QueryClientContext.Provider, { value: props.client }, props.children)
}

export function useQueryClient() {
  const client = useContext(QueryClientContext)

  if (!client) {
    throw new Error('No QueryClient set, use QueryClientProvider to set one')
  }

  return client
}

export function useQuery<TData>(options: QueryOptions<TData>): QueryResult<TData> {
  const client = useQueryClient()
  const queryKeyHash = useMemo(() => stableSerialize(options.queryKey), [options.queryKey])
  const queryKeyRef = useRef(options.queryKey)
  const queryFnRef = useRef(options.queryFn)
  const retryRef = useRef(options.retry)
  const enabledRef = useRef(options.enabled !== false)
  const requestIDRef = useRef(0)
  const mountedRef = useRef(true)
  const dataRef = useRef<TData | undefined>(undefined)
  const [state, setState] = useState<{
    data: TData | undefined
    error: unknown
    status: QueryStatus
    fetchStatus: FetchStatus
  }>({
    data: undefined,
    error: null,
    status: 'pending',
    fetchStatus: 'idle'
  })

  queryKeyRef.current = options.queryKey
  queryFnRef.current = options.queryFn
  retryRef.current = options.retry
  enabledRef.current = options.enabled !== false
  dataRef.current = state.data

  useEffect(() => {
    mountedRef.current = true

    return () => {
      mountedRef.current = false
      requestIDRef.current += 1
    }
  }, [])

  const refetch = useCallback(async () => {
    if (!enabledRef.current) {
      return dataRef.current
    }

    const requestID = requestIDRef.current + 1
    requestIDRef.current = requestID

    setState((prev) => ({
      ...prev,
      error: prev.data === undefined ? null : prev.error,
      status: prev.data === undefined ? 'pending' : prev.status,
      fetchStatus: 'fetching'
    }))

    try {
      const data = await runWithRetry(() => queryFnRef.current(), retryRef.current)
      if (!mountedRef.current || requestID !== requestIDRef.current) {
        return data
      }

      setState({
        data,
        error: null,
        status: 'success',
        fetchStatus: 'idle'
      })

      return data
    } catch (error) {
      if (!mountedRef.current || requestID !== requestIDRef.current) {
        return dataRef.current
      }

      setState((prev) => ({
        data: prev.data,
        error,
        status: 'error',
        fetchStatus: 'idle'
      }))

      throw error
    }
  }, [])

  useEffect(() => {
    setState({
      data: undefined,
      error: null,
      status: 'pending',
      fetchStatus: 'idle'
    })
  }, [queryKeyHash])

  useEffect(() => {
    const unregister = client.registerQuery(queryKeyRef.current, refetch)

    if (options.enabled !== false) {
      refetch().catch(() => undefined)
    }

    return unregister
  }, [client, options.enabled, queryKeyHash, refetch])

  return {
    data: state.data,
    error: state.error,
    status: state.status,
    fetchStatus: state.fetchStatus,
    isLoading: state.status === 'pending' && state.fetchStatus === 'fetching',
    refetch
  }
}

export function useMutation<TData = unknown, TVariables = void>(
  options: MutationOptions<TData, TVariables>
): MutationResult<TData, TVariables> {
  const mutationFnRef = useRef(options.mutationFn)
  const onSuccessRef = useRef(options.onSuccess)
  const onErrorRef = useRef(options.onError)
  const onSettledRef = useRef(options.onSettled)
  const [state, setState] = useState<{
    data: TData | undefined
    error: unknown
    status: MutationStatus
  }>({
    data: undefined,
    error: null,
    status: 'idle'
  })

  mutationFnRef.current = options.mutationFn
  onSuccessRef.current = options.onSuccess
  onErrorRef.current = options.onError
  onSettledRef.current = options.onSettled

  const mutateAsync = useCallback(async (variables: TVariables) => {
    setState({
      data: undefined,
      error: null,
      status: 'pending'
    })

    try {
      const data = await Promise.resolve(mutationFnRef.current(variables))
      setState({
        data,
        error: null,
        status: 'success'
      })

      await onSuccessRef.current?.(data, variables)
      await onSettledRef.current?.(data, null, variables)

      return data
    } catch (error) {
      setState({
        data: undefined,
        error,
        status: 'error'
      })

      await onErrorRef.current?.(error, variables)
      await onSettledRef.current?.(undefined, error, variables)

      throw error
    }
  }, [])

  const mutate = useCallback(
    (variables: TVariables) => {
      mutateAsync(variables).catch(() => undefined)
    },
    [mutateAsync]
  )

  const reset = useCallback(() => {
    setState({
      data: undefined,
      error: null,
      status: 'idle'
    })
  }, [])

  return {
    data: state.data,
    error: state.error,
    status: state.status,
    isPending: state.status === 'pending',
    mutate,
    mutateAsync,
    reset
  }
}
