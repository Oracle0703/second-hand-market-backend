import React, { useEffect } from 'react'
import { QueryClient, QueryClientProvider } from '@/libs/react-query'
import { useDidHide, useDidShow } from '@tarojs/taro'
import { pauseAnalytics, resumeAnalytics } from './services/analytics'
import PrivacyAuthorizationDialog from './components/PrivacyAuthorizationDialog'
import { hydrateSessionStore } from './stores/session'
import './styles/app.scss'

const queryClient = new QueryClient()

const App = (props: React.PropsWithChildren) => {
  useDidShow(resumeAnalytics)
  useDidHide(pauseAnalytics)
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
