import React from 'react'
import { QueryClient, QueryClientProvider } from '@/libs/react-query'
import { ensureDeviceID } from './stores/session'
import './styles/app.scss'

const queryClient = new QueryClient()

ensureDeviceID()

const App = (props) => <QueryClientProvider client={queryClient}>{props.children}</QueryClientProvider>

export default App
