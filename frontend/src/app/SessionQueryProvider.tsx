import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { type ReactNode } from 'react'
import { useAuthStore } from '../stores/auth-store'

let queryClient = new QueryClient()
// Rotate synchronously before React renders the next identity. Clearing also
// cancels pending query retries; late results belong to the discarded client.
useAuthStore.subscribe((state, previous) => {
  if (state.sessionVersion !== previous.sessionVersion) {
    queryClient.clear()
    queryClient = new QueryClient()
  }
})

export function SessionQueryProvider({ children }: { children: ReactNode }) {
  const version = useAuthStore((state) => state.sessionVersion)
  return <QueryClientProvider key={version} client={queryClient}>{children}</QueryClientProvider>
}
