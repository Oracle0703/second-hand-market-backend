// @vitest-environment jsdom
import React from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { act } from 'react-dom/test-utils'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { QueryClient, QueryClientProvider, useQuery } from '../src/libs/react-query'

;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = true
let root: Root
let container: HTMLDivElement
let client: QueryClient
beforeEach(() => { container = document.createElement('div'); document.body.append(container); root = createRoot(container); client = new QueryClient() })
afterEach(async () => { await act(async () => root.unmount()); container.remove() })
function Probe({ identity, enabled, load }: { identity: string; enabled: boolean; load: () => Promise<string> }) {
  const query = useQuery({ queryKey: ['private', identity], enabled, queryFn: load })
  return React.createElement('span', null, query.data ?? 'empty')
}
async function render(identity: string, enabled: boolean, load: () => Promise<string>) {
  await act(async () => root.render(React.createElement(QueryClientProvider, { client }, React.createElement(Probe, { identity, enabled, load }))))
}

test('late previous-identity result cannot populate a disabled new query', async () => {
  let complete!: (value: string) => void
  const loadOld = () => new Promise<string>((resolve) => { complete = resolve })
  await render('old', true, loadOld)
  const loadNew = vi.fn(async () => 'new')
  await render('new', false, loadNew)
  await act(async () => complete('old-private-data'))
  expect(container.textContent).toBe('empty')
  expect(loadNew).not.toHaveBeenCalled()
  await render('new', true, loadNew)
  expect(container.textContent).toBe('new')
})

test('invalidation refreshes mounted matching keys, and unmount removes registrations', async () => {
  const load = vi.fn().mockResolvedValueOnce('before').mockResolvedValue('after')
  await render('user', true, load)
  expect(container.textContent).toBe('before')
  await act(async () => client.invalidateQueries({ queryKey: ['other'] }))
  expect(load).toHaveBeenCalledTimes(1)
  await act(async () => client.invalidateQueries({ queryKey: ['private'] }))
  expect(container.textContent).toBe('after')
  await act(async () => root.render(null))
  await act(async () => client.invalidateQueries({ queryKey: ['private'] }))
  expect(load).toHaveBeenCalledTimes(2)
})
