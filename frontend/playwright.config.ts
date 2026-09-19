import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  testMatch: '**/*.spec.ts',
  fullyParallel: false,
  workers: 1,
  timeout: 90_000,
  expect: { timeout: 15_000 },
  reporter: [['list'], ['html', { open: 'never' }]],
  use: { baseURL: 'http://127.0.0.1:19081', trace: 'retain-on-failure', screenshot: 'only-on-failure', viewport: { width: 1365, height: 900 } },
  webServer: [
    {
      command: 'go run ./scripts/e2e_server',
      cwd: '../backend',
      env: { E2E_FIXTURE: '1' },
      url: 'http://127.0.0.1:19080/healthz',
      reuseExistingServer: false,
      timeout: 120_000,
      gracefulShutdown: { signal: 'SIGTERM', timeout: 10_000 }
    },
    {
      command: 'npm run dev -- --host 127.0.0.1 --port 19081 --strictPort',
      env: { VITE_API_PROXY_TARGET: 'http://127.0.0.1:19080', VITE_API_BASE_URL: '/api/v1' },
      url: 'http://127.0.0.1:19081/login',
      reuseExistingServer: false,
      timeout: 60_000,
      gracefulShutdown: { signal: 'SIGTERM', timeout: 10_000 }
    }
  ]
})
