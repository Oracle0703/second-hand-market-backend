import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, test } from 'vitest'

const productCard = readFileSync(resolve(__dirname, '../src/components/ProductCard.tsx'), 'utf8')
const homePage = readFileSync(resolve(__dirname, '../src/pages/home/index.tsx'), 'utf8')
const categoryPage = readFileSync(resolve(__dirname, '../src/pages/category/index.tsx'), 'utf8')
const detailPage = readFileSync(resolve(__dirname, '../src/pages/product/detail/index.tsx'), 'utf8')

describe('商品图片按需加载', () => {
  test('列表商品图启用原生懒加载', () => {
    expect(productCard).toMatch(/<Image[^>]*lazyLoad/s)
    expect(homePage).toMatch(/<Image[^>]*className="home-product-cover"[^>]*lazyLoad/s)
    expect(categoryPage).toMatch(/<Image[^>]*className="category-product-cover"[^>]*lazyLoad/s)
  })

  test('详情轮播只给当前和相邻项真实图片 URL', () => {
    expect(detailPage).toContain('const [activeImageIndex, setActiveImageIndex] = React.useState(0)')
    expect(detailPage).toMatch(/<Swiper[\s\S]*current=\{activeImageIndex\}[\s\S]*onChange=\{\(event\) => setActiveImageIndex\(event.detail.current\)\}/)
    expect(detailPage).toContain('const shouldLoadImage = Math.abs(idx - activeImageIndex) <= 1')
    expect(detailPage).toContain("src={shouldLoadImage ? url : ''}")
    expect(detailPage).toContain('Taro.previewImage({ current: url, urls: imageURLs })')
  })
})
