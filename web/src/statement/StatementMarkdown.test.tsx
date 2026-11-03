import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { StatementMarkdown } from './StatementMarkdown'
import { safeStatementURL, statementLimits } from './budgets'

const render = (source: string) => renderToStaticMarkup(<StatementMarkdown source={source} />)
describe('shared statement rendering', () => {
  it('renders GFM, inline/display/fenced math, and preserves code as code', () => {
    const html = render('# Title\n\n| a | b |\n| - | - |\n| 1 | 2 |\n\n$x^2$\n\n$$\n\\sum_{i=0}^n i\n$$\n\n```math\na+b\n```\n\n`$not math$`')
    expect(html).toContain('<table>')
    expect(html.match(/class="katex"/g)).toHaveLength(3)
    expect(html).toContain('<code>$not math$</code>')
    expect(html).toContain('application/x-tex')
  })
  it('drops raw HTML, blocks images and dangerous links without embedding elements', () => {
    const html = render('<script>alert(1)</script>\n\n<img src="https://evil.test/a">\n\n![secret](https://evil.test/x)\n\n[x](javascript:alert%281%29) [ok](https://example.test/a)\n\n$\\includegraphics{https://evil.test/z}$')
    expect(html).not.toMatch(/<(script|img|iframe|object)\b/)
    expect(html).not.toContain('href="javascript:')
    expect(html).toContain('图片暂不可用：secret')
    expect(html).toContain('referrerPolicy="no-referrer"')
    expect(html).toContain('href="https://example.test/a"')
  })
  it('contains malformed formula errors and isolates gdef macros between formulas', () => {
    const html = render('$\\badcommand$ then $x+1$\n\n$\\gdef\\testmacro{LEAK}$\n\n$\\testmacro$')
    expect(html).toContain('katex-error')
    expect(html).toContain('公式无法显示：\\badcommand')
    expect(html).toContain('class="katex"')
    expect(html).not.toContain('>LEAK<')
  })
  it('enforces byte, depth, formula count/size/aggregate, and output budgets', () => {
    expect(render('中'.repeat(statementLimits.bytes / 3 + 1))).toContain('128 KiB')
    expect(render('> '.repeat(70) + 'deep')).toContain('题面超过显示预算')
    expect(render('```math\n' + 'x'.repeat(4097) + '\n```')).toContain('公式超过显示预算')
    expect(render(Array.from({ length: 129 }, () => '$x$').join(' '))).toContain('公式超过显示预算')
    expect(render(Array.from({ length: 10 }, () => '$' + 'x'.repeat(4000) + '$').join(' '))).toContain('题面超过显示预算')
  })
  it('does not accept inherited trust options (patched KaTeX)', () => {
    Object.defineProperty(Object.prototype, 'trust', { configurable: true, value: true })
    try { expect(render('$\\href{javascript:alert(1)}{click}$')).not.toContain('href="javascript:') }
    finally { Reflect.deleteProperty(Object.prototype, 'trust') }
  })
  it.each(['javascript:alert(1)', 'data:text/html,x', '//evil.test/x', '/\\evil.test', 'https://user:pass@example.test', 'https://a.test/\n'])('rejects URL %s', value => expect(safeStatementURL(value)).toBe(''))
})
