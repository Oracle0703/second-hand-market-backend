import { defineConfig } from '@tarojs/cli'
import devConfig from './dev'
import prodConfig from './prod'

const TaroEnv = process.env.TARO_ENV || 'weapp'
const IsDev = process.env.NODE_ENV !== 'production'
const ProductionAPIBaseURL = 'https://market.meaningful.ink/api/api/v1'
const DefaultAPIBaseURL = TaroEnv === 'tt' || !IsDev ? ProductionAPIBaseURL : 'http://localhost:8080/api/v1'
const APIBaseURL = process.env.TARO_APP_API_BASE_URL || DefaultAPIBaseURL

export default defineConfig({
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
    __DEV_MODE__: JSON.stringify(IsDev)
  },
  alias: {
    '@': require('path').resolve(__dirname, '..', 'src')
  }
}, process.env.NODE_ENV === 'development' ? devConfig : prodConfig)
