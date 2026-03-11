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
    'pages/me/index',
    'pages/intent/create/index',
    'pages/intent/list/index'
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
        text: '首页'
      },
      {
        pagePath: 'pages/category/index',
        text: '分类'
      },
      {
        pagePath: 'pages/favorite/index',
        text: '收藏'
      },
      {
        pagePath: 'pages/me/index',
        text: '我的'
      }
    ]
  }
}

export default config
