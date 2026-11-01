import { useCallback, useEffect, useRef, useState } from 'react'
import type { ChangeEvent, KeyboardEvent, PointerEvent } from 'react'
import { Link, useParams } from 'react-router'
import { useCurrentAccount } from '../account/context'
import { NotFoundPage } from '../pages/StatusPages'
import { Icon } from '../ui/Icon'
import { CodeEditor } from './CodeEditor'
import { decodePreviewCode, downloadPreviewCode, previewCodeLimit } from './codeFiles'
import { ProblemStatement } from './ProblemStatement'
import { previewProblems } from './problems'
import { usePreviewDrafts } from './draftContext'
import type { PreviewProblem } from './problems'
import './workspace.css'

const starter = '#include <iostream>\nusing namespace std;\n\nint main() {\n    // 在这里编写代码\n    return 0;\n}\n'
const clamp = (value: number) => Math.max(30, Math.min(65, value))

function ProblemWorkspace({ problem }: { problem: PreviewProblem }) {
  const { documents, updateDocument } = usePreviewDrafts()
  const code = documents[problem.id] ?? starter
  const dirty = Object.hasOwn(documents, problem.id)
  const [width, setWidth] = useState(46)
  const [notice, setNotice] = useState('')
  const revision = useRef(0)
  const reads = useRef(0)
  const active = useRef(false)
  const layout = useRef<HTMLDivElement>(null)
  const input = useRef<HTMLInputElement>(null)
  const onChange = useCallback((value: string) => { ++revision.current; updateDocument(problem.id, value); setNotice('') }, [problem.id, updateDocument])
  useEffect(() => {
    active.current = true
    return () => { active.current = false }
  }, [])

  async function upload(event: ChangeEvent<HTMLInputElement>) {
    const file = event.currentTarget.files?.[0]
    event.currentTarget.value = ''
    if (!file) return
    if (!/\.(cpp|cc|cxx|txt)$/i.test(file.name) || file.size > previewCodeLimit) {
      setNotice('请选择不超过 128 KiB 的 .cpp、.cc、.cxx 或 .txt 文件。')
      return
    }
    if (dirty && !window.confirm('上传会替换当前预览代码。请确认已下载需要保留的内容。')) return
    const request = ++reads.current
    const expected = revision.current
    try {
      const value = decodePreviewCode(await file.arrayBuffer())
      if (!active.current || request !== reads.current) return
      if (revision.current !== expected) { setNotice('读取期间代码已改变，未替换。请重新选择文件。'); return }
      ++revision.current
      updateDocument(problem.id, value); setNotice('文件已载入预览编辑器，尚未提交。')
    } catch { if (active.current && request === reads.current && revision.current === expected) setNotice('文件无法作为 UTF-8 文本载入，或超过 128 KiB。当前代码已保留。') }
  }

  function resize(event: PointerEvent<HTMLDivElement>) {
    if (!event.currentTarget.hasPointerCapture(event.pointerId) || !layout.current) return
    const box = layout.current.getBoundingClientRect()
    setWidth(clamp((event.clientX - box.left) / box.width * 100))
  }
  function resizeKey(event: KeyboardEvent<HTMLDivElement>) {
    const values: Record<string, number> = { ArrowLeft: width - 2, ArrowRight: width + 2, Home: 30, End: 65 }
    if (event.key in values) { event.preventDefault(); setWidth(clamp(values[event.key])) }
  }
  function release(event: PointerEvent<HTMLDivElement>) {
    if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId)
  }
  return <div className="workspace" ref={layout} style={{ gridTemplateColumns: `${width}fr 6px ${100 - width}fr` }}>
    <ProblemStatement problem={problem} />
    <div role="separator" aria-label="调整题面宽度" aria-orientation="vertical" aria-valuemin={30} aria-valuemax={65} aria-valuenow={Math.round(width)} tabIndex={0} className="pane-separator" onPointerDown={event => event.currentTarget.setPointerCapture(event.pointerId)} onPointerMove={resize} onPointerUp={release} onPointerCancel={release} onKeyDown={resizeKey} />
    <section className="editor-pane" aria-label="代码编辑预览">
      <div className="editor-toolbar"><label htmlFor="preview-language">语言预览</label><select id="preview-language" defaultValue="cpp"><option value="cpp">GNU++17</option></select><div className="editor-file-actions"><input ref={input} type="file" accept=".cpp,.cc,.cxx,.txt" aria-label="选择预览代码文件" hidden onChange={event => void upload(event)} /><button className="button-neutral" type="button" onClick={() => input.current?.click()}><Icon name="upload" />上传代码</button><button type="button" className="button-neutral" onClick={() => { downloadPreviewCode(problem.id, code); setNotice('已创建代码下载，请确认浏览器中的下载结果。') }}><Icon name="download" />下载代码</button></div></div>
      <CodeEditor value={code} onChange={onChange} />
      {notice ? <p role="status" className="editor-notice">{notice}</p> : null}
      <div className="editor-actions"><p id="preview-submit-note">示例代码仅在此标签页临时保留，刷新或切换账号前请下载。</p><button type="button" disabled aria-describedby="preview-submit-note" title="正式判题尚未开放">提交代码</button></div>
      <details className="submission-result"><summary><span>提交结果</span><small>尚未开放</small></summary><div className="disclosure-content"><p>当前为界面预览，未连接正式判题服务。编辑、上传和下载均不会发送代码或创建提交记录。</p></div></details>
    </section>
  </div>
}

export default function ProblemPage() {
  const { id } = useParams()
  const { state } = useCurrentAccount()
  if (state.status === 'checking' || state.status === 'unknown') return <section className="page-width status-page"><h1>做题页预览</h1><p role="status">{state.status === 'checking' ? '正在核对账号状态，核对完成后即可编辑。' : '账号状态尚未确认，请先检查当前会话，再打开预览编辑器。'}</p><Link className="button button-neutral" to="/account">检查账号状态</Link></section>
  const problem = previewProblems.find(item => item.id === id)
  return problem ? <ProblemWorkspace key={problem.id} problem={problem} /> : <NotFoundPage />
}
