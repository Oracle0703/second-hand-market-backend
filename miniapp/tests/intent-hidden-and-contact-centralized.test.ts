import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, test } from 'vitest'
import appConfig from '../src/app.config'

const mePageSource = readFileSync(resolve(__dirname, '../src/pages/me/index.tsx'), 'utf8')
const detailPageSource = readFileSync(resolve(__dirname, '../src/pages/product/detail/index.tsx'), 'utf8')
const favoritePageSource = readFileSync(resolve(__dirname, '../src/pages/favorite/index.tsx'), 'utf8')
const homePageSource = readFileSync(resolve(__dirname, '../src/pages/home/index.tsx'), 'utf8')
const categoryPageSource = readFileSync(resolve(__dirname, '../src/pages/category/index.tsx'), 'utf8')

describe('意向入口隐藏与电话集中管理', () => {
  test('小程序页面注册中不再暴露意向页面', () => {
    expect(appConfig.pages).not.toContain('pages/intent/create/index')
    expect(appConfig.pages).not.toContain('pages/intent/list/index')
  })

  test('我的页面不再展示意向入口', () => {
    expect(mePageSource).not.toContain('/pages/intent/list/index')
    expect(mePageSource).not.toContain('未关闭意向')
  })

  test('详情页改为我想要，收藏页也提供我想要入口', () => {
    expect(detailPageSource).toContain('我想要')
    expect(detailPageSource).not.toContain('提交意向')
    expect(favoritePageSource).toContain('我想要')
  })

  test('页面不再各自维护硬编码电话号码', () => {
    expect(homePageSource).not.toContain("const PHONE_NUMBER = '13699479406'")
    expect(categoryPageSource).not.toContain("const PHONE_NUMBER = '13699479406'")
    expect(homePageSource).toContain('promptAndCallStore')
    expect(categoryPageSource).toContain('promptAndCallStore')
    expect(detailPageSource).toContain('promptAndCallStore')
    expect(favoritePageSource).toContain('promptAndCallStore')
  })
})
