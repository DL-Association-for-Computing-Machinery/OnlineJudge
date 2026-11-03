import rehypeKatex from 'rehype-katex'
import type { Element, Root } from 'hast'
import { SKIP, visitParents } from 'unist-util-visit-parents'

type MathFile = Parameters<ReturnType<typeof rehypeKatex>>[1]
export function rehypeRestrictedMath() {
  return (tree: Root, file: MathFile) => {
    visitParents(tree, 'element', (node: Element, parents) => {
      const classes = node.properties.className
      if (!Array.isArray(classes) || !classes.some(value => ['language-math', 'math-display', 'math-inline'].includes(String(value)))) return
      let scope = node
      let parent = parents.at(-1)
      if (classes.includes('language-math') && node.tagName === 'code' && parent?.type === 'element' && parent.tagName === 'pre') {
        scope = parent
        parent = parents.at(-2)
      }
      if (!parent || (parent.type !== 'element' && parent.type !== 'root')) return
      const index = parent.children.indexOf(scope)
      const original = node.children.map(child => child.type === 'text' ? child.value : '').join('')
      const isolated: Root = { type: 'root', children: [scope] }
      const before = file.messages.length
      // Construct per formula: even global definitions cannot leak to the next
      // formula. The upstream transformer handles fenced/display/inline math.
      rehypeKatex({ trust: false, maxExpand: 128, maxSize: 20, strict: 'error' })(isolated, file)
      const result: Root['children'] = file.messages.length === before ? isolated.children : [{
        type: 'element', tagName: 'span', properties: { className: ['katex-error'] },
        children: [{ type: 'text', value: `公式无法显示：${original}` }],
      }]
      parent.children.splice(index, 1, ...result)
      return SKIP
    })
  }
}
