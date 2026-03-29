type MiniProgramProvider = 'wechat' | 'douyin'

type MiniProgramPlatform = {
  provider: MiniProgramProvider
  loginTitle: string
  defaultNickname: string
}

function resolveTaroEnv(): string {
  if (typeof process !== 'undefined' && process.env && typeof process.env.TARO_ENV === 'string') {
    return process.env.TARO_ENV
  }
  return 'weapp'
}

export function getMiniProgramPlatform(): MiniProgramPlatform {
  const env = resolveTaroEnv()
  if (env === 'tt') {
    return {
      provider: 'douyin',
      loginTitle: '抖音登录',
      defaultNickname: '抖音买家'
    }
  }

  return {
    provider: 'wechat',
    loginTitle: '微信登录',
    defaultNickname: '微信买家'
  }
}
