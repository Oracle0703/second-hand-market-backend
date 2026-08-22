export const ERROR_MESSAGES: Record<number, string> = {
  10001: '参数校验失败',
  10002: '登录已失效，请重新登录',
  10003: '无权限访问',
  10004: '资源不存在',
  10005: '当前状态下不可执行该操作',
  10006: '当前账号处于入驻受限状态，仅可访问入驻流程功能',
  10007: '账号已禁用',
  10008: '上传文件不合法',
  10009: '请求过于频繁',
  10010: '商品已被其他订单占用',
  10011: '重复提交'
}

const UPLOAD_ERROR_MESSAGES: Record<string, Partial<Record<number, string>>> = {
  '/files/presign': {
    10001: '图片文件信息异常，请重新选择图片后再上传',
    10008: '图片格式或大小不支持，请上传 JPG、PNG、WebP、HEIC、HEIF，原图不超过 40MB'
  },
  '/files/upload': {
    10001: '图片上传参数已失效，请重新选择图片后再上传',
    10008: '图片处理失败，请换一张图片，或先降低分辨率/压缩后再上传'
  },
  '/files/confirm': {
    10001: '图片上传确认信息异常，请重新选择图片后再上传',
    10008: '图片上传未完成，请重新选择图片后再上传'
  }
}

export function apiErrorMessage(code: number, path?: string, fallback?: string) {
  const uploadMessage = UPLOAD_ERROR_MESSAGES[path ?? '']?.[code]
  if (uploadMessage) return uploadMessage
  return ERROR_MESSAGES[code] ?? fallback ?? '请求失败，请稍后重试'
}
