const config = {
  pages: [
    'pages/home/index',
    'pages/category/index',
    'pages/search/index',
    'pages/product/list/index',
    'pages/product/detail/index',
    'pages/favorite/index',
    'pages/history/index',
    'pages/login/index',
    'pages/store-guide/index',
    'pages/me/index'
  ],
  window: {
    backgroundTextStyle: 'light',
    navigationBarBackgroundColor: '#ffffff',
    navigationBarTitleText: '二手好物',
    navigationBarTextStyle: 'black'
  },
  tabBar: {
    color: '#888',
    selectedColor: '#1d5a4a',
    backgroundColor: '#ffffff',
    borderStyle: 'black',
    list: [
      {
        pagePath: 'pages/home/index',
        text: '首页',
        iconPath: 'assets/tabbar/home.png',
        selectedIconPath: 'assets/tabbar/home-active.png'
      },
      {
        pagePath: 'pages/category/index',
        text: '分类',
        iconPath: 'assets/tabbar/category.png',
        selectedIconPath: 'assets/tabbar/category-active.png'
      },
      {
        pagePath: 'pages/favorite/index',
        text: '收藏',
        iconPath: 'assets/tabbar/favorite.png',
        selectedIconPath: 'assets/tabbar/favorite-active.png'
      }
    ]
  }
}

export default config
