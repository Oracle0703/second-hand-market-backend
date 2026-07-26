export const MAX_UPLOAD_FILE_BYTES = 10 * 1024 * 1024

export function validateUploadFile(file: File) {
  if (file.size <= 0) return '图片文件不能为空'
  if (file.size > MAX_UPLOAD_FILE_BYTES) return '图片不能超过 10 MiB'
  return null
}
