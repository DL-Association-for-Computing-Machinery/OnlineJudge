export const previewCodeLimit = 128 * 1024

export function decodePreviewCode(bytes: ArrayBuffer): string {
  if (bytes.byteLength > previewCodeLimit) throw new Error('预览文件不能超过 128 KiB。')
  let code: string
  try { code = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(bytes) }
  catch { throw new Error('请选择 UTF-8 编码的代码文件。') }
  if (code.includes('\0')) throw new Error('文件包含非文本内容，未替换代码。')
  return code
}

export function downloadPreviewCode(id: string, code: string) {
  const url = URL.createObjectURL(new Blob([code], { type: 'text/plain;charset=utf-8' }))
  const link = document.createElement('a')
  link.href = url
  link.download = `ecut-oj-preview-${id}.cpp`
  document.body.append(link)
  link.click()
  link.remove()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}
