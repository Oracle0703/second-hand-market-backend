import React, { useEffect } from 'react'
import { QueryClient, QueryClientProvider } from '@/libs/react-query'
import PrivacyAuthorizationDialog from './components/PrivacyAuthorizationDialog'
import { hydrateSessionStore } from './stores/session'
import './styles/app.scss'

const queryClient = new QueryClient()

const App = (props) => {
  useEffect(() => {
    hydrateSessionStore()
  }, [])

  return (
    <QueryClientProvider client={queryClient}>
      {props.children}
      <PrivacyAuthorizationDialog />
    </QueryClientProvider>
  )
}

export default App
