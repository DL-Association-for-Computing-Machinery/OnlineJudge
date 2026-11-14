import type { TrainingLanguage } from '../services/trainingHistory'

export const trainingSourceLimit = 65_536
const encoder = new TextEncoder()

export function validTrainingSource(source: string) {
  return source.length > 0 && !source.includes('\0') && !/[\uD800-\uDFFF]/u.test(source) &&
    encoder.encode(source).length <= trainingSourceLimit
}

export async function readTrainingSource(file: File, language: TrainingLanguage = 'gnu++17') {
  if (language === 'c17' && !/\.c$/i.test(file.name)) throw new Error('请选择 .c 文件。')
  if (language !== 'c17' && !/\.(cpp|cc|cxx)$/i.test(file.name)) throw new Error('请选择 .cpp、.cc 或 .cxx 文件。')
  if (file.size > trainingSourceLimit) throw new Error('源文件不能超过 64 KiB。')
  let source: string
  try {
    source = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(await file.arrayBuffer())
  } catch {
    throw new Error('无法读取文件，请选择 UTF-8 编码的源文件。')
  }
  if (!validTrainingSource(source)) throw new Error('源文件为空或含无效字符。')
  return source
}
