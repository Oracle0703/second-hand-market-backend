import { defineConfig } from '@tarojs/cli'
import devConfig from './dev'
import prodConfig from './prod'
import { resolveAPIBaseURL } from './api-base-url'

const TaroEnv = (process.env.TARO_ENV || 'weapp').trim().toLowerCase()
const NodeEnv = (process.env.NODE_ENV || 'development').trim().toLowerCase()
const IsDev = NodeEnv !== 'production'
const analyticsURL = (process.env.TARO_APP_ANALYTICS_ENDPOINT || '').trim()
const analyticsProjectKey = (process.env.TARO_APP_ANALYTICS_PROJECT_KEY || '').trim()
if (Boolean(analyticsURL) !== Boolean(analyticsProjectKey) || /\s/.test(analyticsProjectKey)) {
  throw new Error('Set TARO_APP_ANALYTICS_ENDPOINT and TARO_APP_ANALYTICS_PROJECT_KEY together, using a project write key without spaces')
}
const analyticsEndpoint = analyticsURL
  ? resolveAPIBaseURL({ taroEnv: TaroEnv, nodeEnv: NodeEnv, envBaseURL: analyticsURL })
  : ''

const APIBaseURL = resolveAPIBaseURL({
  taroEnv: TaroEnv,
  nodeEnv: NodeEnv,
  envBaseURL: process.env.TARO_APP_API_BASE_URL
})

export default defineConfig((merge) =>
  merge({}, {
    projectName: 'second-hand-buyer-miniapp',
    date: '2026-03-11',
    designWidth: 750,
    deviceRatio: {
      640: 2.34 / 2,
      750: 1,
      828: 1.81 / 2
    },
    sourceRoot: 'src',
    outputRoot: `dist/${TaroEnv}`,
    framework: 'react',
    compiler: {
      type: 'webpack5',
      prebundle: {
        enable: false
      }
    },
    mini: {
      postcss: {
        pxtransform: {
          enable: true,
          config: {}
        },
        url: {
          enable: true,
          config: {
            limit: 1024
          }
        },
        cssModules: {
          enable: false,
          config: {
            namingPattern: 'module',
            generateScopedName: '[name]__[local]___[hash:base64:5]'
          }
        }
      }
    },
    plugins: [],
    defineConstants: {
      __API_BASE_URL__: JSON.stringify(APIBaseURL),
      __EVENT_ANALYTICS_ENDPOINT__: JSON.stringify(analyticsEndpoint),
      __EVENT_ANALYTICS_PROJECT_KEY__: JSON.stringify(analyticsProjectKey),
      __MERCHANT_NO__: JSON.stringify(process.env.TARO_APP_MERCHANT_NO || ''),
      __DEV_MODE__: JSON.stringify(IsDev)
    },
    alias: {
      '@': require('path').resolve(__dirname, '..', 'src')
    }
  }, IsDev ? devConfig : prodConfig)
)
