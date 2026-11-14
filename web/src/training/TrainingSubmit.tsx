import { useEffect, useRef, useState, type FormEvent } from 'react'
import { Link } from 'react-router'
import { useCurrentAccount } from '../account/context'
import { useCodeDraftSession } from '../preview/useCodeDraftSession'
import { subscribePrivateInvalidation } from '../account/privateInvalidation'
import { recoverTraining, submitTraining, TrainingIntakeError, type TrainingAttempt } from '../services/trainingIntake'
import { isTrainingLanguage, trainingLanguageLabel, type SubmissionSummary, type TrainingLanguage } from '../services/trainingHistory'
import { clearTrainingKey, readTrainingKey, reserveTrainingKey, trainingReceiptSlot } from './receiptJournal'
import { readTrainingSource, validTrainingSource } from './sourceFile'
import { TrainingDraftControls } from './TrainingDraftControls'
import './trainingSubmit.css'

export function TrainingSubmit({ problemID, version }: { problemID: string; version: string }) {
  const { state } = useCurrentAccount()
  if (state.status === 'anonymous') return <p><Link to="/login">登录后提交</Link></p>
  if (state.status !== 'authenticated' || !state.session) return null
  return <TrainingLanguageForm key={state.session.subject_id + problemID + version} subject={state.session.subject_id}
    problemID={problemID} version={version} accountBusy={state.busy} />
}

type TrainingFormProps = { subject: string; problemID: string; version: string; accountBusy: boolean }

function TrainingLanguageForm(props: TrainingFormProps) {
  const [language, setLanguage] = useState<TrainingLanguage>('gnu++17')
  return <TrainingForm key={language} {...props} language={language} setLanguage={setLanguage} />
}

function TrainingForm({ subject, problemID, version, accountBusy, language, setLanguage }: TrainingFormProps & {
  language: TrainingLanguage; setLanguage: (language: TrainingLanguage) => void
}) {
  const { controller } = useCurrentAccount()
  const draftSession = useCodeDraftSession(subject, problemID + ':' + version, '', 'personal-training', language)
  const { draft, state: draftState, verifying } = draftSession
  const source = draftState.code
  const editorUnavailable = verifying || draftState.phase === 'cleared' || draftState.phase === 'loading'
  const slot = trainingReceiptSlot(subject, problemID)
  const [initialKey] = useState(() => {
    try { return readTrainingKey(slot) }
    catch { return undefined }
  })
  const [key, setKey] = useState(initialKey ?? null)
  const journalReady = initialKey !== undefined
  const [canRetry, setCanRetry] = useState(false)
  const [confirmForget, setConfirmForget] = useState(false)
  const [selectedFile, setSelectedFile] = useState<File | null>(null)
  const [fileBusy, setFileBusy] = useState(false)
  const [busy, setBusy] = useState(false)
  const [notice, setNotice] = useState(!journalReady ? '无法读取本标签页的提交回执，暂时不能提交。' :
    initialKey ? '有一笔提交尚未确认。请查询原回执。' : '')
  const [receipt, setReceipt] = useState<SubmissionSummary>()
  const attempt = useRef<TrainingAttempt | null>(null)
  const request = useRef<AbortController | null>(null)
  const fileRead = useRef<AbortController | null>(null)
  useEffect(() => {
    const clear = () => {
      request.current?.abort()
      request.current = null
      attempt.current = null
      fileRead.current?.abort()
      fileRead.current = null
    }
    const unsubscribe = subscribePrivateInvalidation(() => {
      clear()
      setSelectedFile(null)
      setFileBusy(false)
      setCanRetry(false)
      setConfirmForget(false)
      setBusy(false)
      setNotice('会话状态已变化，请重新检查后再操作。')
      setReceipt(undefined)
    })
    return () => { unsubscribe(); clear() }
  }, [])

  const invalid = !validTrainingSource(source)

  async function run(recover: boolean) {
    const snapshot = controller.getSnapshot()
    if (!journalReady || request.current || fileRead.current || snapshot.busy || snapshot.status !== 'authenticated' ||
      snapshot.session?.subject_id !== subject) return
    const deadline = Math.min(Date.parse(snapshot.session.idle_expires_at), Date.parse(snapshot.session.absolute_deadline))
    if (!Number.isFinite(deadline) || Date.now() >= deadline) return
    const generation = controller.readPrivateGeneration()
    const stillCurrent = () => {
      const current = controller.getSnapshot()
      return controller.readPrivateGeneration() === generation && current.status === 'authenticated' &&
        current.session?.subject_id === subject && Date.now() < deadline
    }
    let original: string | null
    try {
      original = readTrainingKey(slot)
      if (!original && !recover && !editorUnavailable && !invalid && !selectedFile && window.isSecureContext) {
        const candidate = crypto.randomUUID()
        if (!reserveTrainingKey(slot, candidate)) return
        original = candidate
        attempt.current = { subject, key: candidate, problemID, version, language, source }
        setCanRetry(true)
        setSelectedFile(null)
        setReceipt(undefined)
      }
    } catch {
      setNotice('无法保存原回执，本次未发出请求。')
      return
    }
    setKey(original)
    if (!original) return
    const intent = attempt.current
    if (!recover && intent?.key !== original) {
      setCanRetry(false)
      setNotice('请查询原回执。')
      return
    }
    if (!recover) {
      setReceipt(undefined)
    }
    const abort = new AbortController()
    request.current = abort
    setBusy(true)
    setNotice(recover ? '正在查询原提交…' : '正在提交…')
    try {
      const result = recover ? await recoverTraining({ subject, key: original }, abort.signal) :
        intent && await submitTraining(intent, abort.signal)
      if (abort.signal.aborted || !stillCurrent()) return
      if (!result) return
      setReceipt(result)
      setNotice('已收到提交，可在「我的提交」查看结果。')
      attempt.current = null
      setCanRetry(false)
      setConfirmForget(false)
      try {
        setKey(clearTrainingKey(slot, original) ? null : readTrainingKey(slot))
      } catch {
        setNotice('已收到提交，但浏览器未能清除回执记录。')
      }
    } catch (error) {
      if (abort.signal.aborted || !stillCurrent()) return
      const status = error instanceof TrainingIntakeError ? error.status : 0
      // A missing receipt may race an in-flight commit. Keep the original
      // key and source even when a subsequent attempt receives a rejection.
      setNotice(recover && status === 404 ? intent ? '暂未查到回执，可以再次查询或用原请求重试。' :
        '暂未查到回执。原请求可能仍在处理，请稍后再查。' :
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

  function forgetReceipt() {
    const snapshot = controller.getSnapshot()
    if (!key || !confirmForget || request.current || snapshot.busy || snapshot.status !== 'authenticated' ||
      snapshot.session?.subject_id !== subject) return
    try {
      clearTrainingKey(slot, key)
      setKey(readTrainingKey(slot))
      attempt.current = null
      setCanRetry(false)
      setConfirmForget(false)
      setNotice('已清除此标签页的原回执。已接收的提交仍可在「我的提交」查看。')
    } catch {
      setNotice('无法清除原回执，请先查看「我的提交」。')
    }
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!key) void run(false)
  }

  async function loadFile(file: File) {
    const snapshot = controller.getSnapshot()
    if (editorUnavailable || key || request.current || fileRead.current || snapshot.busy || snapshot.status !== 'authenticated' ||
      snapshot.session?.subject_id !== subject) return
    const abort = new AbortController()
    fileRead.current = abort
    setFileBusy(true)
    setSelectedFile(null)
    try {
      const text = await readTrainingSource(file, language)
      if (abort.signal.aborted || controller.getSnapshot().session?.subject_id !== subject) return
      setNotice(draft.edit(text) ? '已载入源码文件，确认后可提交。' : '会话正在核对，文件未替换。')
    } catch (error) {
      if (!abort.signal.aborted) setNotice(error instanceof Error ? error.message : '无法读取文件。')
    } finally {
      if (fileRead.current === abort) {
        fileRead.current = null
        setFileBusy(false)
      }
    }
  }

  return <section className="training-submit" aria-labelledby="training-submit-heading">
    <h2 id="training-submit-heading">提交代码</h2>
    <form onSubmit={submit}>
      <label htmlFor="training-language">语言</label>
      <select id="training-language" value={language} disabled={!journalReady || Boolean(key) || busy || fileBusy || accountBusy}
        onChange={event => {
          const value = event.target.value
          if (isTrainingLanguage(value)) setLanguage(value)
        }}>
        {Object.entries(trainingLanguageLabel).filter(([key]) =>
          (key !== 'java21' || import.meta.env.VITE_OJ_LOCAL_JAVA21 === '1') &&
          (key !== 'python3' || import.meta.env.VITE_OJ_LOCAL_PYTHON3 === '1'))
          .map(([key, label]) => <option key={key} value={key}>{label}</option>)}
      </select>
      <p>源码最多 64 KiB</p>
      {language === 'java21' ? <p>请使用无包名的 Main 类作为程序入口。</p> : null}
      <label htmlFor="training-source">源码</label>
      <textarea id="training-source" rows={10} maxLength={65_536} spellCheck={false}
        value={editorUnavailable ? '' : source} readOnly={editorUnavailable || Boolean(key) || fileBusy}
        onChange={event => { draft.edit(event.target.value) }} />
      {source && invalid ? <p role="status">源码含无效字符或超过 64 KiB。</p> : null}
      {!key ? <>
        <label htmlFor="training-source-file">选择源码文件</label>
        <input id="training-source-file" type="file" accept={language === 'python3' ? '.py' : language === 'java21' ? '.java' : language === 'c17' ? '.c' : '.cpp,.cc,.cxx'} disabled={editorUnavailable || busy || fileBusy || accountBusy}
          onChange={event => {
            const file = event.target.files?.[0]
            event.target.value = ''
            if (!file) return
            if (source) setSelectedFile(file)
            else void loadFile(file)
          }} />
        {selectedFile ? <div className="history-controls">
          <p>编辑区已有内容。替换后，原内容将丢失。</p>
          <button type="button" className="button button-outline" disabled={busy || accountBusy}
            onClick={() => void loadFile(selectedFile)}>替换编辑区内容</button>
          <button type="button" className="button button-outline" onClick={() => setSelectedFile(null)}>保留当前源码</button>
        </div> : null}
      </> : null}
      <div className="history-controls">
        {key ? <>
          <button type="button" className="button button-neutral" disabled={busy || accountBusy} onClick={() => void run(true)}>查询原回执</button>
          {canRetry ? <button type="button" className="button button-outline" disabled={busy || accountBusy} onClick={() => void run(false)}>重试原提交</button> : null}
          <button type="button" className="button button-outline" disabled={busy || accountBusy}
            onClick={() => setConfirmForget(true)}>清除本地回执</button>
        </> : <button type="submit" className="button button-neutral"
          disabled={editorUnavailable || !journalReady || busy || fileBusy || accountBusy || Boolean(selectedFile) || invalid}>提交判题</button>}
      </div>
      {key && confirmForget ? <div className="history-controls">
        <p>清除不会取消原请求。请先查看「我的提交」；再次提交可能产生重复记录。</p>
        <button type="button" className="button button-outline" disabled={busy || accountBusy}
          onClick={forgetReceipt}>确认清除回执</button>
        <button type="button" className="button button-outline" onClick={() => setConfirmForget(false)}>保留原回执</button>
      </div> : null}
    </form>
    <TrainingDraftControls session={draftSession} filename={problemID} locked={Boolean(key) || busy || fileBusy || accountBusy} />
    <p role="status">{notice}</p>
    {receipt ? <Link to={'/submissions?submission=' + receipt.id}>查看本次提交</Link> : null}
  </section>
}
