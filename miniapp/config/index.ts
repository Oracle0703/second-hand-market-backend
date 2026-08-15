import { defineConfig } from '@tarojs/cli'
import devConfig from './dev'
import prodConfig from './prod'
import { resolveAPIBaseURL } from './api-base-url'

const TaroEnv = (process.env.TARO_ENV || 'weapp').trim().toLowerCase()
const NodeEnv = (process.env.NODE_ENV || 'development').trim().toLowerCase()
const IsDev = NodeEnv !== 'production'

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
      __MERCHANT_NO__: JSON.stringify(process.env.TARO_APP_MERCHANT_NO || ''),
      __DEV_MODE__: JSON.stringify(IsDev)
    },
    alias: {
      '@': require('path').resolve(__dirname, '..', 'src')
    }
  }, IsDev ? devConfig : prodConfig)
)
