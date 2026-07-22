import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, test } from 'vitest'

const miniappRoot = resolve(__dirname, '..')
const packageJSON = JSON.parse(readFileSync(resolve(miniappRoot, 'package.json'), 'utf8'))
const packageLock = JSON.parse(readFileSync(resolve(miniappRoot, 'package-lock.json'), 'utf8'))

describe('小程序构建工具链版本', () => {
  test('固定 Node、npm 和 Babel runtime 插件版本', () => {
    expect(readFileSync(resolve(miniappRoot, '.nvmrc'), 'utf8').trim()).toBe('22.22.2')
    expect(packageJSON.packageManager).toBe('npm@10.9.7')
    expect(packageJSON.engines).toEqual({ node: '22.22.2', npm: '10.9.7' })
    expect(packageJSON.devDependencies['@babel/core']).toBe('7.29.0')
    expect(packageJSON.devDependencies['@babel/plugin-transform-runtime']).toBe('7.26.10')
    expect(packageJSON.overrides['@babel/plugin-transform-runtime']).toBe('$@babel/plugin-transform-runtime')
    expect(packageLock.packages['node_modules/@babel/plugin-transform-runtime'].version).toBe('7.26.10')
  })
})
