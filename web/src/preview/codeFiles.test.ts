import { describe, expect, it } from 'vitest'
import { decodePreviewCode, previewCodeLimit } from './codeFiles'

describe('preview code import boundaries', () => {
  it('preserves whitespace, line endings and Unicode source text', () => {
    const code = '\ufeff  // 中文 e\u0301\r\n\tint main() {}\r\n'
    expect(decodePreviewCode(new TextEncoder().encode(code).buffer)).toBe(code)
  })
  it('accepts the size boundary and rejects the next byte before decoding', () => {
    expect(decodePreviewCode(new Uint8Array(previewCodeLimit).fill(32).buffer).length).toBe(previewCodeLimit)
    expect(() => decodePreviewCode(new Uint8Array(previewCodeLimit + 1).fill(32).buffer)).toThrow('128 KiB')
  })
  it('rejects malformed UTF-8 rather than inserting replacement characters', () => {
    expect(() => decodePreviewCode(new Uint8Array([0xc3, 0x28]).buffer)).toThrow('UTF-8')
  })
  it('rejects binary and UTF-16-like NUL content', () => {
    expect(() => decodePreviewCode(new Uint8Array([65, 0, 66, 0]).buffer)).toThrow('非文本')
  })
})
