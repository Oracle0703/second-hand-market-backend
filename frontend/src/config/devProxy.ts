export function createDevProxy(target: string) {
  return {
    '/api': {
      target,
      changeOrigin: true
    },
    '/uploads': {
      target,
      changeOrigin: true
    }
  }
}
