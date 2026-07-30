import { afterEach, describe, expect, test, vi } from 'vitest'
import { resolveAPIBaseURL } from '../config/api-base-url'

const miniAppTargets = ['weapp', 'tt'] as const
const label63 = 'a'.repeat(63)
const label64 = 'a'.repeat(64)
const hostname253 = `${'a'.repeat(63)}.${'b'.repeat(63)}.${'c'.repeat(63)}.${'d'.repeat(61)}`
const hostname254 = `${'a'.repeat(63)}.${'b'.repeat(63)}.${'c'.repeat(63)}.${'d'.repeat(62)}`

type ConfigLayer = {
  outputRoot?: string
  defineConstants?: Record<string, string>
  env?: Record<string, string>
}

afterEach(() => {
  vi.unstubAllEnvs()
})

async function probeBuildConfig(taroEnv: typeof miniAppTargets[number]): Promise<ConfigLayer[]> {
  vi.resetModules()
  vi.stubEnv('TARO_ENV', taroEnv)
  vi.stubEnv('NODE_ENV', 'production')
  vi.stubEnv('TARO_APP_API_BASE_URL', 'https://api.example.com/api/v1')

  const { default: configFactory } = await import('../config/index')
  if (typeof configFactory !== 'function') {
    throw new Error('expected Taro config to use the defineConfig factory form')
  }

  const mergeProbe = (...layers: Array<object | null | undefined>): object =>
    layers as unknown as object
  const result = await configFactory(mergeProbe, { command: 'build', mode: 'production' })
  return result as unknown as ConfigLayer[]
}

describe('小程序 API 基址选择', () => {
  test.each(miniAppTargets)('%s 开发构建未设置覆盖时使用本机安全默认值', (taroEnv) => {
    expect(
      resolveAPIBaseURL({
        taroEnv,
        nodeEnv: 'development'
      })
    ).toBe('http://127.0.0.1:8080/api/v1')
  })

  test.each([
    'http://localhost:8080/api/v1',
    'http://127.0.0.1:18080/api/v1',
    'http://[::1]:8080/api/v1',
    'http://10.20.30.40:8080/api/v1',
    'https://172.16.1.20/api/v1',
    'http://192.168.50.20:8080/api/v1',
    'https://[fd12:3456:789a::20]:8443/api/v1'
  ])('微信和抖音开发构建接受受控本地地址并得到相同结果：%s', (envBaseURL) => {
    const results = miniAppTargets.map((taroEnv) =>
      resolveAPIBaseURL({
        taroEnv,
        nodeEnv: 'development',
        envBaseURL
      })
    )

    expect(results).toEqual([envBaseURL, envBaseURL])
  })

  test.each(miniAppTargets)('%s production 未设置覆盖时保持正式 API 地址', (taroEnv) => {
    expect(
      resolveAPIBaseURL({
        taroEnv,
        nodeEnv: 'production'
      })
    ).toBe('https://market.meaningful.ink/api/v1')
  })

  test('production 接受公开 HTTPS 覆盖且微信和抖音结果一致', () => {
    const envBaseURL = 'https://api.example.com/api/v1'
    const results = miniAppTargets.map((taroEnv) =>
      resolveAPIBaseURL({
        taroEnv,
        nodeEnv: ' Production ',
        envBaseURL
      })
    )

    expect(results).toEqual([envBaseURL, envBaseURL])
  })

  test.each([
    ['普通公网 DNS hostname', 'https://api.example.com/api/v1'],
    ['中间连字符 label', 'https://api-v2.example.com/api/v1'],
    ['单个 DNS root 尾点', 'https://api.example.com./api/v1'],
    ['63 字符 label', `https://${label63}.example.com/api/v1`],
    ['253 字符完整 hostname', `https://${hostname253}/api/v1`],
    ['Unicode IDN hostname', 'https://bücher.example/api/v1'],
    ['punycode IDN hostname', 'https://xn--bcher-kva.example/api/v1']
  ])('production 接受合法公网 DNS hostname：%s', (_name, envBaseURL) => {
    const results = miniAppTargets.map((taroEnv) =>
      resolveAPIBaseURL({
        taroEnv,
        nodeEnv: 'production',
        envBaseURL
      })
    )

    expect(results).toEqual([envBaseURL, envBaseURL])
  })

  test.each([
    'https://8.8.8.8/api/v1',
    'https://[2606:4700:4700::1111]/api/v1'
  ])('production 接受公开 HTTPS IP 地址：%s', (envBaseURL) => {
    expect(
      resolveAPIBaseURL({
        taroEnv: 'weapp',
        nodeEnv: 'production',
        envBaseURL
      })
    ).toBe(envBaseURL)
  })

  test('development 接受显式公开 HTTPS 覆盖但仍拒绝公开 HTTP', () => {
    const envBaseURL = 'https://api.example.com/api/v1'
    const results = miniAppTargets.map((taroEnv) =>
      resolveAPIBaseURL({
        taroEnv,
        nodeEnv: 'development',
        envBaseURL
      })
    )

    expect(results).toEqual([envBaseURL, envBaseURL])
    expect(() =>
      resolveAPIBaseURL({
        taroEnv: 'weapp',
        nodeEnv: 'development',
        envBaseURL: 'http://api.example.com/api/v1'
      })
    ).toThrow(/TARO_APP_API_BASE_URL/)
  })

  test.each([
    'http://api.example.com/api/v1',
    'http://localhost:8080/api/v1',
    'https://service.localhost/api/v1',
    'https://localhost./api/v1',
    'https://127.0.0.1/api/v1',
    'https://127.1/api/v1',
    'https://2130706433/api/v1',
    'https://0x7f000001/api/v1',
    'https://0177.0.0.1/api/v1',
    'https://[::1]/api/v1',
    'https://[::ffff:127.0.0.1]/api/v1',
    'https://[::ffff:0:127.0.0.1]/api/v1',
    'https://[64:ff9b::7f00:1]/api/v1',
    'https://[2002:7f00:1::]/api/v1',
    'https://10.0.0.1/api/v1',
    'https://172.16.0.1/api/v1',
    'https://172.31.255.255/api/v1',
    'https://192.168.1.1/api/v1',
    'https://169.254.1.1/api/v1',
    'https://100.64.0.1/api/v1',
    'https://[fc00::1]/api/v1',
    'https://[fd12:3456::1]/api/v1',
    'https://[fe80::1]/api/v1',
    'https://api.local/api/v1',
    'https://printer.office.local./api/v1',
    'https://internal/api/v1'
  ])('production 构建拒绝非公开或非 HTTPS 地址：%s', (envBaseURL) => {
    for (const taroEnv of miniAppTargets) {
      expect(() =>
        resolveAPIBaseURL({
          taroEnv,
          nodeEnv: 'production',
          envBaseURL
        })
      ).toThrow(/TARO_APP_API_BASE_URL/)
    }
  })

  test.each([
    'https://bad_host.example.com/api/v1',
    'https://api..example.com/api/v1',
    'https://.api.example.com/api/v1',
    'https://api.example.com../api/v1',
    'https://-api.example.com/api/v1',
    'https://api-.example.com/api/v1',
    `https://${label64}.example.com/api/v1`,
    `https://${hostname254}/api/v1`
  ])('production 构建拒绝非法 DNS hostname 语法：%s', (envBaseURL) => {
    for (const taroEnv of miniAppTargets) {
      expect(() =>
        resolveAPIBaseURL({
          taroEnv,
          nodeEnv: 'production',
          envBaseURL
        })
      ).toThrow(/TARO_APP_API_BASE_URL/)
    }
  })

  test('非法 DNS hostname 错误不回显原始 URL 或 sentinel', () => {
    const sentinel = 'sentinel-secret-hostname'
    const envBaseURL = `https://bad_host.${sentinel}.example.com/api/v1`

    try {
      resolveAPIBaseURL({
        taroEnv: 'weapp',
        nodeEnv: 'production',
        envBaseURL
      })
      throw new Error('expected production DNS hostname validation to fail')
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error)
      expect(message).toMatch(/TARO_APP_API_BASE_URL/)
      expect(message).not.toContain(envBaseURL)
      expect(message).not.toContain(sentinel)
      expect(message).not.toContain('bad_host')
    }
  })

  test.each([
    'http://169.254.1.1:8080/api/v1',
    'http://100.64.0.1:8080/api/v1',
    'http://device.local:8080/api/v1',
    'http://0.0.0.0:8080/api/v1',
    'https://0.0.0.0:8443/api/v1',
    'http://172.15.255.255:8080/api/v1',
    'http://172.32.0.1:8080/api/v1'
  ])('development 构建拒绝非 loopback/RFC1918/ULA 地址：%s', (envBaseURL) => {
    expect(() =>
      resolveAPIBaseURL({
        taroEnv: 'weapp',
        nodeEnv: 'development',
        envBaseURL
      })
    ).toThrow(/TARO_APP_API_BASE_URL/)
  })

  test.each([
    'not-a-url',
    'file:///tmp/api',
    'https://user:password@example.com/api/v1',
    'https://api.example.com/api/v1?tenant=local',
    'https://api.example.com/api/v1#local',
    'https://api.example.com:0/api/v1',
    'https://api.example.com:65536/api/v1',
    'https://api.example.com\\@127.0.0.1/api/v1',
    'https://api.example.com/api/\nv1'
  ])('任一构建模式都拒绝含糊或危险的 URL 语法：%s', (envBaseURL) => {
    expect(() =>
      resolveAPIBaseURL({
        taroEnv: 'tt',
        nodeEnv: 'production',
        envBaseURL
      })
    ).toThrow(/TARO_APP_API_BASE_URL/)
  })

  test('空白开发覆盖不回退正式 API', () => {
    expect(
      resolveAPIBaseURL({
        taroEnv: 'weapp',
        nodeEnv: 'development',
        envBaseURL: '   '
      })
    ).toBe('http://127.0.0.1:8080/api/v1')
  })

  test('未知 NODE_ENV 失败关闭', () => {
    expect(() =>
      resolveAPIBaseURL({
        taroEnv: 'weapp',
        nodeEnv: 'staging',
        envBaseURL: 'https://api.example.com/api/v1'
      })
    ).toThrow(/NODE_ENV/)
  })
})

describe('Taro 构建配置接线', () => {
  test('微信和抖音 production 构建注入相同的已校验 API 地址', async () => {
    const weappLayers = await probeBuildConfig('weapp')
    const ttLayers = await probeBuildConfig('tt')

    expect(weappLayers[1].outputRoot).toBe('dist/weapp')
    expect(ttLayers[1].outputRoot).toBe('dist/tt')
    expect(weappLayers[1].defineConstants?.__API_BASE_URL__).toBe(
      JSON.stringify('https://api.example.com/api/v1')
    )
    expect(ttLayers[1].defineConstants).toEqual(weappLayers[1].defineConstants)
    expect(weappLayers[2].env?.NODE_ENV).toBe('production')
    expect(ttLayers[2].env?.NODE_ENV).toBe('production')
  })

  test.each(miniAppTargets)('%s 危险 production 覆盖在配置模块加载阶段失败', async (taroEnv) => {
    vi.resetModules()
    vi.stubEnv('TARO_ENV', taroEnv)
    vi.stubEnv('NODE_ENV', 'production')
    vi.stubEnv('TARO_APP_API_BASE_URL', 'http://192.168.50.20:8080/api/v1')

    await expect(import('../config/index')).rejects.toThrow(
      /TARO_APP_API_BASE_URL must use HTTPS in production/
    )
  })
})
