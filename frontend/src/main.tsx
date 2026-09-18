import React from 'react'
import ReactDOM from 'react-dom/client'
import { SessionQueryProvider } from './app/SessionQueryProvider'
import { App } from './app/App'
import 'antd/dist/reset.css'
import './styles/global.css'


ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <SessionQueryProvider>
      <App />
    </SessionQueryProvider>
  </React.StrictMode>
)
