import { defineConfig } from '@tarojs/cli'
import devConfig from './dev'
import prodConfig from './prod'

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
  outputRoot: 'dist',
  framework: 'react',
  compiler: 'webpack5',
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
  alias: {
    '@': require('path').resolve(__dirname, '..', 'src')
  }
}, process.env.NODE_ENV === 'development' ? devConfig : prodConfig)
