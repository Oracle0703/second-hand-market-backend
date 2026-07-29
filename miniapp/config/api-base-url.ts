const ProductionAPIBaseURL = 'https://market.meaningful.ink/api/v1'
const LocalAPIBaseURL = 'http://localhost:8080/api/v1'

export function resolveAPIBaseURL(options: {
  taroEnv: string
  nodeEnv: string
  envBaseURL?: string
}): string {
  if (options.envBaseURL?.trim()) {
    return options.envBaseURL.trim()
  }

  const isMiniApp = options.taroEnv === 'weapp' || options.taroEnv === 'tt'
  const isDev = options.nodeEnv !== 'production'

  return isMiniApp || !isDev ? ProductionAPIBaseURL : LocalAPIBaseURL
}
