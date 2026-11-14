import { validImageSource, type StatementImage } from './images'

export type StatementPayload<Field extends 'draft_id' | 'problem_id'> = { [Key in Field]: string } & {
  version: string
  title: string
  language: 'en'
  markdown: string
  examples: { id: string; input: string; output: string }[]
  images?: StatementImage[]
}
export const validStatementID = (id: string) => /^[a-f0-9]{32}$/.test(id)
const encoder = new TextEncoder()
const decoder = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true })
const record = (value: unknown): value is Record<string, unknown> => typeof value === 'object' && value !== null && !Array.isArray(value)

function text(value: unknown): value is string {
  return typeof value === 'string' && decoder.decode(encoder.encode(value)) === value
}

export function decodeStatement<Field extends 'draft_id' | 'problem_id'>(
  value: unknown, id: string, idField: Field, invalid: () => Error,
): StatementPayload<Field> {
  if (!record(value) || Object.keys(value).some(key => ![idField, 'version', 'title', 'language', 'markdown', 'examples', 'images'].includes(key)) ||
    Object.keys(value).length !== ('images' in value ? 7 : 6) || value[idField] !== id || !validStatementID(id) ||
    typeof value.version !== 'string' || !/^[a-f0-9]{64}$/.test(value.version) || !text(value.title) || !value.title || encoder.encode(value.title).byteLength > 512 || value.language !== 'en' ||
    !text(value.markdown) || !Array.isArray(value.examples) || value.examples.length > 1024) throw invalid()
  let bytes = encoder.encode(value.markdown).byteLength
  const ids = new Set<string>()
  for (const example of value.examples) {
    if (!record(example) || Object.keys(example).length !== 3 || typeof example.id !== 'string' || !/^case-\d{4}$/.test(example.id) || ids.has(example.id) || !text(example.input) || !text(example.output)) throw invalid()
    ids.add(example.id)
    bytes += encoder.encode(example.input).byteLength + encoder.encode(example.output).byteLength
  }
  if ('images' in value) {
    if (!Array.isArray(value.images) || value.images.length < 1 || value.images.length > 16) throw invalid()
    const sources = new Set<string>()
    let pixels = 0
    for (const image of value.images) {
      if (!record(image) || Object.keys(image).length !== 5 || typeof image.source !== 'string' || !validImageSource(image.source) || sources.has(image.source) ||
        image.media_type !== 'image/png' || typeof image.data !== 'string' || image.data.length > 349_528 ||
        typeof image.width !== 'number' || !Number.isInteger(image.width) || image.width < 1 || image.width > 4096 ||
        typeof image.height !== 'number' || !Number.isInteger(image.height) || image.height < 1 || image.height > 4096 || image.width * image.height > 4_000_000) throw invalid()
      let decoded: string
      try { decoded = atob(image.data) } catch { throw invalid() }
      if (decoded.length < 24 || decoded.length > 262_144 || btoa(decoded) !== image.data || !image.data.startsWith('iVBORw0KGgo')) throw invalid()
      const dimension = (offset: number) => Array.from(decoded.slice(offset, offset + 4)).reduce((n, c) => n * 256 + c.charCodeAt(0), 0)
      if (decoded.slice(12, 16) !== 'IHDR' || dimension(16) !== image.width || dimension(20) !== image.height) throw invalid()
      sources.add(image.source)
      bytes += decoded.length
      pixels += image.width * image.height
      if (pixels > 8_000_000) throw invalid()
    }
  }
  if (bytes > 1_048_576) throw invalid()
  return value as unknown as StatementPayload<Field>
}
