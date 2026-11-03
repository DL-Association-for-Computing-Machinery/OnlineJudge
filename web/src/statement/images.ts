export interface StatementImage {
  source: string
  media_type: 'image/png'
  data: string
  width: number
  height: number
}

export function validImageSource(source: string): boolean {
  return source.length <= 240 && !/[^A-Za-z0-9_./-]/.test(source) && ['.png', '.jpg', '.jpeg'].some(extension => source.endsWith(extension)) &&
    source.split('/').every(part => part.length > 0 && !part.startsWith('.'))
}
