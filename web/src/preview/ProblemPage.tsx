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
import { useCodeDraftSession } from './useCodeDraftSession'
import type { PreviewProblem } from './problems'
import './workspace.css'

const starter = '#include <iostream>\nusing namespace std;\n\nint main() {\n    // 在这里编写代码\n    return 0;\n}\n'
const clamp = (value: number) => Math.max(30, Math.min(65, value))

interface WorkspaceDraft {
  code: string
  dirty: boolean
  update: (code: string) => boolean
  controls?: React.ReactNode
  disabled?: boolean
}

function GuestWorkspace({ problem }: { problem: PreviewProblem }) {
  const { documents, updateDocument } = usePreviewDrafts()
  const update = useCallback((code: string) => { updateDocument(problem.id, code); return true }, [problem.id, updateDocument])
  return <ProblemWorkspace problem={problem} draft={{ code: documents[problem.id] ?? starter,
    dirty: Object.hasOwn(documents, problem.id), update }} />
}

function PrivateWorkspace({ problem, owner }: { problem: PreviewProblem; owner: string }) {
  const { draft, state, verifying, verifyFailed } = useCodeDraftSession(owner, problem.id, starter)
  const update = useCallback((code: string) => draft.edit(code), [draft])
  const messages = {
    loading: '正在读取此账号的本地草稿…', ready: '没有可恢复的本地草稿；浏览器不支持跨设备恢复。',
    dirty: '代码有尚未保存的更改。', saving: '正在保存…', saved: '已保存到此浏览器。',
    conflict: '此草稿已有另一版本。可下载两份代码，或明确选择要保留的版本。',
    failed: '本地保存或读取失败。当前代码尚未确认保存，请下载备份后重试。',
    cleared: '当前账号状态已变化。请返回账号页确认会话后重新打开。',
  }
  const disabled = verifying || state.phase === 'loading' || state.phase === 'cleared'
  return <ProblemWorkspace problem={problem} draft={{
    code: state.code, dirty: state.phase !== 'ready', update, disabled,
    controls: <div className="editor-notice" aria-label="本地草稿">
      <p role="status">{verifyFailed ? '暂时无法核对会话。恢复连接并重新聚焦页面后会再检查。' : verifying ? '正在核对当前会话…' : messages[state.phase]}</p>
      {!verifying && state.phase === 'failed' ? <button type="button" className="button-neutral" onClick={() => draft.retry()}>重试本地保存</button> : null}
      {state.phase === 'cleared' ? <Link to="/account">检查账号状态</Link> : null}
      {!verifying && state.phase === 'conflict' ? <>
        <button type="button" className="button-neutral" onClick={() => draft.chooseDraft(state.code)}>保留当前编辑</button>
        {state.stored !== undefined ? <>
          <button type="button" className="button-neutral" onClick={() => downloadPreviewCode(problem.id, state.stored ?? '')}>下载已存版本</button>
          <button type="button" className="button-neutral" onClick={() => draft.chooseDraft(state.stored ?? '')}>保留已存版本</button>
        </> : null}
      </> : null}
      {!disabled ? state.alternatives.map((alternative, index) => <div key={alternative.id}>
        冲突副本 {index + 1}
        <button type="button" className="button-neutral" onClick={() => downloadPreviewCode(problem.id, alternative.draft.code)}>下载副本</button>
        <button type="button" className="button-neutral" onClick={() => {
          if (window.confirm('使用此副本会替换当前草稿。请先下载需要保留的版本。')) draft.chooseDraft(alternative.draft.code, alternative.id)
        }}>使用副本</button>
      </div>) : null}
    </div>,
  }} />
}

function ProblemWorkspace({ problem, draft }: { problem: PreviewProblem; draft: WorkspaceDraft }) {
  const { code, dirty, update, disabled, controls } = draft
  const [width, setWidth] = useState(46)
  const [notice, setNotice] = useState('')
  const revision = useRef(0)
  const reads = useRef(0)
  const active = useRef(false)
  const layout = useRef<HTMLDivElement>(null)
  const input = useRef<HTMLInputElement>(null)
  const onChange = useCallback((value: string) => { ++revision.current; update(value); setNotice('') }, [update])
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
      setNotice(update(value) ? '文件已载入预览编辑器，尚未提交。' : '会话正在核对，文件未替换。请在编辑器恢复后重新选择。')
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
      <div className="editor-toolbar"><label htmlFor="preview-language">语言预览</label><select id="preview-language" defaultValue="cpp"><option value="cpp">GNU++17</option></select><div className="editor-file-actions"><input ref={input} type="file" accept=".cpp,.cc,.cxx,.txt" aria-label="选择预览代码文件" hidden onChange={event => void upload(event)} /><button className="button-neutral" type="button" disabled={disabled} onClick={() => input.current?.click()}><Icon name="upload" />上传代码</button><button type="button" className="button-neutral" disabled={disabled} onClick={() => { downloadPreviewCode(problem.id, code); setNotice('已创建代码下载，请确认浏览器中的下载结果。') }}><Icon name="download" />下载代码</button></div></div>
      {!disabled ? <CodeEditor value={code} onChange={onChange} /> : null}
      {notice ? <p role="status" className="editor-notice">{notice}</p> : null}
      {controls}
      <div className="editor-actions"><p id="preview-submit-note">{controls ? '草稿仅存于当前浏览器，不会提交或同步。' : '未登录时，示例代码仅在此标签页临时保留。'}</p><button type="button" disabled aria-describedby="preview-submit-note" title="正式判题尚未开放">提交代码</button></div>
      <details className="submission-result"><summary><span>提交结果</span><small>尚未开放</small></summary><div className="disclosure-content"><p>当前为界面预览，未连接正式判题服务。编辑、上传和下载均不会发送代码或创建提交记录。</p></div></details>
    </section>
  </div>
}

export default function ProblemPage() {
  const { id } = useParams()
  const { state } = useCurrentAccount()
  if (state.status === 'checking' || state.status === 'unknown') return <section className="page-width status-page"><h1>做题页预览</h1><p role="status">{state.status === 'checking' ? '正在核对账号状态，核对完成后即可编辑。' : '账号状态尚未确认，请先检查当前会话，再打开预览编辑器。'}</p><Link className="button button-neutral" to="/account">检查账号状态</Link></section>
  const problem = previewProblems.find(item => item.id === id)
  if (!problem) return <NotFoundPage />
  const owner = state.status === 'authenticated' ? state.session?.subject_id : undefined
  return owner ? <PrivateWorkspace key={owner + ':' + problem.id} owner={owner} problem={problem} /> :
    <GuestWorkspace key={problem.id} problem={problem} />
}
