import { useEffect, useRef, useState, type FormEvent } from 'react'
import { Link } from 'react-router'
import { useCurrentAccount } from '../account/context'
import { subscribePrivateInvalidation } from '../account/privateInvalidation'
import { requestTraining, TrainingIntakeError, type TrainingAttempt } from '../services/trainingIntake'
import type { SubmissionSummary } from '../services/trainingHistory'
import './trainingSubmit.css'

const sourceEncoder = new TextEncoder()

export function TrainingSubmit({ problemID, version }: { problemID: string; version: string }) {
  const { state } = useCurrentAccount()
  if (state.status === 'anonymous') return <p><Link to="/login">登录后提交</Link></p>
  if (state.status !== 'authenticated' || !state.session) return null
  return <TrainingForm key={state.session.subject_id + version} subject={state.session.subject_id}
    problemID={problemID} version={version} accountBusy={state.busy} />
}

function TrainingForm({ subject, problemID, version, accountBusy }: {
  subject: string; problemID: string; version: string; accountBusy: boolean
}) {
  const { controller } = useCurrentAccount()
  const [source, setSource] = useState('')
  const [pending, setPending] = useState(false)
  const [busy, setBusy] = useState(false)
  const [notice, setNotice] = useState('')
  const [receipt, setReceipt] = useState<SubmissionSummary>()
  const attempt = useRef<TrainingAttempt | null>(null)
  const request = useRef<AbortController | null>(null)
  useEffect(() => {
    const clear = () => {
      request.current?.abort()
      request.current = null
      attempt.current = null
    }
    const unsubscribe = subscribePrivateInvalidation(() => {
      clear()
      setSource('')
      setPending(false)
      setBusy(false)
      setNotice('')
      setReceipt(undefined)
    })
    return () => { unsubscribe(); clear() }
  }, [])

  const invalid = source.length === 0 || source.includes('\0') || sourceEncoder.encode(source).length > 65_536

  async function run(recover: boolean) {
    const snapshot = controller.getSnapshot()
    if (request.current || snapshot.busy || snapshot.status !== 'authenticated' || snapshot.session?.subject_id !== subject) return
    const deadline = Math.min(Date.parse(snapshot.session.idle_expires_at), Date.parse(snapshot.session.absolute_deadline))
    if (!Number.isFinite(deadline) || Date.now() >= deadline) return
    const generation = controller.readPrivateGeneration()
    const stillCurrent = () => {
      const current = controller.getSnapshot()
      return controller.readPrivateGeneration() === generation && current.status === 'authenticated' &&
        current.session?.subject_id === subject && Date.now() < deadline
    }
    if (!attempt.current) {
      if (recover || invalid || !window.isSecureContext) return
      attempt.current = { subject, key: crypto.randomUUID(), problemID, version, source }
      setPending(true)
      setReceipt(undefined)
    }
    const abort = new AbortController()
    request.current = abort
    setBusy(true)
    setNotice(recover ? '正在查询原提交…' : '正在提交…')
    try {
      const result = await requestTraining(attempt.current, recover, abort.signal)
      if (abort.signal.aborted || !stillCurrent()) return
      setReceipt(result)
      setSource('')
      setNotice('已收到提交，可在「我的提交」查看结果。')
      attempt.current = null
      setPending(false)
    } catch (error) {
      if (abort.signal.aborted || !stillCurrent()) return
      const status = error instanceof TrainingIntakeError ? error.status : 0
      // A missing receipt may race an in-flight commit. Keep the original
      // key and source even when a subsequent attempt receives a rejection.
      setNotice(recover && status === 404 ? '暂未查到回执，可以再次查询或用原请求重试。' :
        !recover && status === 429 ? '队列已满，请稍后重试原提交。' :
        status === 401 || status === 403 || status === 409 ? '无法继续确认提交，请检查当前账号。' :
        !recover && (status === 404 || status === 405) ? '此题目版本暂不可提交。可查询原回执，或返回题库。' :
        '尚未确认是否收到提交，请查询原回执或重试原提交。')
    } finally {
      if (request.current === abort) {
        request.current = null
        setBusy(false)
      }
    }
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!pending) void run(false)
  }

  return <section className="training-submit" aria-labelledby="training-submit-heading">
    <h2 id="training-submit-heading">提交代码</h2>
    <form onSubmit={submit}>
      <p>语言：GNU++17 · 源码最多 64 KiB</p>
      <label htmlFor="training-source">源码</label>
      <textarea id="training-source" rows={10} maxLength={65_536} spellCheck={false}
        value={source} readOnly={pending} onChange={event => setSource(event.target.value)} />
      {source && invalid ? <p role="status">源码含无效字符或超过 64 KiB。</p> : null}
      <div className="history-controls">
        {pending ? <>
          <button type="button" className="button button-neutral" disabled={busy || accountBusy} onClick={() => void run(true)}>查询原回执</button>
          <button type="button" className="button button-outline" disabled={busy || accountBusy} onClick={() => void run(false)}>重试原提交</button>
        </> : <button type="submit" className="button button-neutral" disabled={busy || accountBusy || invalid}>提交判题</button>}
      </div>
    </form>
    <p role="status">{notice}</p>
    {receipt ? <Link to={'/submissions?submission=' + receipt.id}>查看本次提交</Link> : null}
  </section>
}
