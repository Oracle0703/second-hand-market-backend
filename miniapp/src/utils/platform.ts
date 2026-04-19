import Taro, { ENV_TYPE } from '@tarojs/taro'

type MiniProgramEnv = 'weapp' | 'tt'
type MiniProgramProvider = 'wechat' | 'douyin'

type MiniProgramPlatform = {
  env: MiniProgramEnv
  provider: MiniProgramProvider
  loginTitle: string
  defaultNickname: string
}

type RuntimeGlobal = {
  tt?: unknown
  wx?: unknown
}

function resolveRuntimeGlobal(): RuntimeGlobal {
  if (typeof globalThis !== 'undefined') {
    return globalThis as RuntimeGlobal
  }
  return {}
}

export function resolveTaroEnv(): MiniProgramEnv {
  try {
    const env = Taro.getEnv?.()
    if (env === ENV_TYPE.TT) {
      return 'tt'
    }
    if (env === ENV_TYPE.WEAPP) {
      return 'weapp'
    }
  } catch {}

  if (typeof process !== 'undefined' && process.env && typeof process.env.TARO_ENV === 'string') {
    return process.env.TARO_ENV === 'tt' ? 'tt' : 'weapp'
  }

  const runtimeGlobal = resolveRuntimeGlobal()
  if (typeof runtimeGlobal.tt !== 'undefined') {
    return 'tt'
  }
  if (typeof runtimeGlobal.wx !== 'undefined') {
    return 'weapp'
  }

  return 'weapp'
}

export function getMiniProgramPlatform(): MiniProgramPlatform {
  const env = resolveTaroEnv()
  if (env === 'tt') {
    return {
      env,
      provider: 'douyin',
      loginTitle: '抖音登录',
      defaultNickname: '抖音买家'
    }
  }

  return {
    env,
    provider: 'wechat',
    loginTitle: '微信登录',
    defaultNickname: '微信买家'
  }
}
