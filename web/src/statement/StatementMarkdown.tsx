import Markdown from 'react-markdown'
import { useEffect, useState } from 'react'
import { ErrorBoundary } from 'react-error-boundary'
import remarkGfm from 'remark-gfm'
import remarkMath from 'remark-math'
import { rehypeRestrictedMath } from './math'
import { byteLength, rehypeImageBudget, rehypeMathBudget, rehypeOutputBudget, remarkBudget, safeStatementURL, statementLimits } from './budgets'
import 'katex/dist/katex.min.css'
import './statement.css'
import type { StatementImage } from './images'

function AuthorizedImage({ image, alt }: { image: StatementImage; alt: string }) {
  const [failed, setFailed] = useState(false)
  return failed ? <span className="statement-image-placeholder">图片无法显示{alt ? `：${alt}` : ''}</span> :
    <img className="statement-authorized-image" src={`data:image/png;base64,${image.data}`} alt={alt} width={image.width} height={image.height} decoding="async" onError={() => setFailed(true)} />
}

function SourceFallback({ source, notice }: { source: string; notice: string }) {
  return <div className="statement-markdown"><p role="status">{notice}</p><details><summary>查看原文{source.length > 32768 ? '节选（过长部分省略）' : ''}</summary><pre>{source.slice(0, 32768)}</pre></details></div>
}

export function StatementMarkdown({ source, images }: { source: string; images?: StatementImage[] }) {
  const authorized = new Map(images?.map(image => [image.source, image]))
  const [fontFailed, setFontFailed] = useState(false)
  useEffect(() => {
    const fonts = document.fonts
    let active = true
    const check = () => {
      if (active && Array.from(fonts).some(font => font.family.includes('KaTeX') && font.status === 'error')) setFontFailed(true)
    }
    fonts.addEventListener('loadingerror', check)
    void fonts.ready.then(check)
    return () => { active = false; fonts.removeEventListener('loadingerror', check) }
  }, [])
  if (byteLength(source) > statementLimits.bytes) return <SourceFallback source={source} notice="题面超过 128 KiB 显示预算，请缩减内容后重试。" />
  return <ErrorBoundary resetKeys={[source]} fallback={<SourceFallback source={source} notice="题面暂时无法显示，请检查 Markdown 与公式。" />}>
    <div className="statement-markdown">
      {fontFailed && <SourceFallback source={source} notice="数学字体未能加载，可查看原文。" />}
      <Markdown skipHtml remarkPlugins={[remarkGfm, remarkMath, remarkBudget]}
        rehypePlugins={[rehypeMathBudget, rehypeRestrictedMath, rehypeOutputBudget, rehypeImageBudget]}
        urlTransform={(url, key) => key === 'src' ? (authorized.has(url) ? url : '') : safeStatementURL(url)}
        components={{
          a: ({ href, children }) => href ? <a href={href} rel="noopener noreferrer" referrerPolicy="no-referrer">{children}</a> : <span>{children}（链接不可用）</span>,
          img: ({ src, alt }) => typeof src === 'string' && authorized.has(src) ? <AuthorizedImage key={src} image={authorized.get(src)!} alt={alt ?? ''} /> : <span className="statement-image-placeholder">图片暂不可用{alt ? `：${alt}` : ''}</span>,
        }}>{source}</Markdown>
    </div>
  </ErrorBoundary>
}
