import type { Root as MarkdownRoot } from 'mdast'
import type { Element, Root as HTMLRoot } from 'hast'
import { EXIT, visitParents } from 'unist-util-visit-parents'

export const statementLimits = { bytes: 131_072, nodes: 20_000, depth: 64, formulaBytes: 4096, formulas: 128, mathBytes: 32_768 } as const
const encoder = new TextEncoder()
export const byteLength = (text: string) => encoder.encode(text).byteLength
const tooComplex = '题面超过显示预算，请查看原文并缩减内容。'

// Parsing and KaTeX run on the main thread. These finite work/output budgets
// limit admitted content; they are not a hard CPU time limit.
function withinBudget(tree: MarkdownRoot | HTMLRoot): boolean {
  let count = 0
  let valid = true
  visitParents(tree, (_node, parents) => {
    if (++count > statementLimits.nodes || parents.length > statementLimits.depth) {
      valid = false
      return EXIT
    }
  })
  return valid
}

export function remarkBudget() {
  return (tree: MarkdownRoot, file: { value: unknown }) => {
    if (!withinBudget(tree)) tree.children = [{ type: 'paragraph', children: [{ type: 'text', value: tooComplex }] }, { type: 'code', value: typeof file.value === 'string' ? file.value : '' }]
  }
}

export function rehypeMathBudget() {
  return (tree: HTMLRoot) => {
    let count = 0
    let bytes = 0
    visitParents(tree, 'element', (node: Element) => {
      const classes = node.properties.className
      if (!Array.isArray(classes) || !classes.some(value => ['language-math', 'math-display', 'math-inline'].includes(String(value)))) return
      // Math plugins produce text-only candidates, including fenced math. Do
      // not render unexpected nested markup as a formula.
      const text = node.children.map(child => child.type === 'text' ? child.value : '').join('')
      const size = byteLength(text)
      bytes += size
      if (++count > statementLimits.formulas || bytes > statementLimits.mathBytes || size > statementLimits.formulaBytes || node.children.some(child => child.type !== 'text')) {
        node.properties = { className: ['math-budget-error'] }
        node.children = [{ type: 'text', value: `公式超过显示预算：${text}` }]
      }
    })
  }
}

export function rehypeOutputBudget() {
  return (tree: HTMLRoot, file: { value: unknown }) => {
    if (!withinBudget(tree)) tree.children = [
      { type: 'element', tagName: 'p', properties: { role: 'status' }, children: [{ type: 'text', value: tooComplex }] },
      { type: 'element', tagName: 'pre', properties: {}, children: [{ type: 'text', value: typeof file.value === 'string' ? file.value : '' }] },
    ]
  }
}

// Asset counts do not bound Markdown occurrences. Excess copies stay as text.
export function rehypeImageBudget() {
  return (tree: HTMLRoot) => {
    let count = 0
    visitParents(tree, 'element', (node: Element) => {
      if (node.tagName === 'img' && ++count > 32) node.properties.src = ''
    })
  }
}

// Links never inherit a private draft referrer. Images use a separate exact
// match against the authorized preview manifest, never these link rules.
export function safeStatementURL(url: string): string {
  if (Array.from(url).some(char => char.charCodeAt(0) <= 32 || (char.charCodeAt(0) >= 127 && char.charCodeAt(0) <= 159) || char === '\\') || url.startsWith('//')) return ''
  if (url.startsWith('#')) return url
  if (url.startsWith('/') && !url.startsWith('//')) return url
  try {
    const parsed = new URL(url)
    return parsed.protocol === 'https:' && !parsed.username && !parsed.password ? parsed.href : ''
  } catch { return '' }
}
