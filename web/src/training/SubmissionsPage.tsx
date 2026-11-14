import { useEffect, useState, useSyncExternalStore } from 'react'
import { Link, useSearchParams } from 'react-router'
import { useCurrentAccount } from '../account/context'
import { subscribePrivateInvalidation } from '../account/privateInvalidation'
import { HistoryController } from './historyController'
import { trainingLanguageLabel, type SubmissionSummary } from '../services/trainingHistory'
import './trainingHistory.css'

const stateLabels = { queued: '排队中', running: '判题中', finished: '已判定', failed: '判题失败' }
const formatDate = new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'medium' })

function HistoryRow({ row, history, detail, accountBusy, linked = false }: {
  row: SubmissionSummary
  history: HistoryController
  detail: ReturnType<HistoryController['getSnapshot']>['detail']
  accountBusy: boolean
  linked?: boolean
}) {
  const expanded = detail?.id === row.id
  const current = expanded && detail.value ? detail.value : row
  const panelID = 'submission-detail-' + row.id
  return <li>
    <div className="history-row-heading">
      <h2>提交 <code>{row.id}</code></h2>
      <span>{stateLabels[current.state]}{current.verdict ? ` · ${current.verdict}` : ''}</span>
    </div>
    <dl>
      <div><dt>接收时间</dt><dd><time dateTime={row.created_at}>{formatDate.format(new Date(row.created_at))}</time></dd></div>
      <div><dt>语言</dt><dd>{trainingLanguageLabel[row.language]}</dd></div>
      <div><dt>题目材料版本</dt><dd><code>{row.material_version}</code></dd></div>
    </dl>
    <button type="button" className="button button-outline" aria-expanded={expanded} aria-controls={expanded ? panelID : undefined}
      disabled={accountBusy} onClick={() => expanded ? history.closeDetail() : void reopen()}>
      {expanded ? '收起提交详情' : '查看提交详情'}
    </button>
    {expanded ? <div id={panelID} className="history-detail">
      <p role="status">{detail.notice || '已读取提交详情。'}</p>
      <button type="button" className="button button-outline" disabled={detail.busy || accountBusy}
        onClick={() => void reopen()}>重新读取详情</button>
      {detail.value ? <>
        <h3 id={panelID + '-source'}>提交时的源码</h3>
        <pre tabIndex={0} aria-labelledby={panelID + '-source'}><code>{detail.value.source}</code></pre>
      </> : null}
    </div> : null}
  </li>

  function reopen() {
    return linked ? history.openLinkedDetail(row.id) : history.openDetail(row.id)
  }
}

export function SubmissionsPage() {
  const { state: account, controller: accountController } = useCurrentAccount()
  const [history] = useState(() => new HistoryController(accountController))
  const state = useSyncExternalStore(history.subscribe, history.getSnapshot)
  const [params] = useSearchParams()
  const id = params.get('submission') ?? ''
  const linked = /^[a-f0-9]{32}$/.test(id)
  useEffect(() => {
    history.connect()
    const unsubscribe = subscribePrivateInvalidation(history.clear)
    for (const event of ['focus', 'pagehide', 'pageshow']) window.addEventListener(event, history.clear)
    document.addEventListener('visibilitychange', history.clear)
    return () => {
      unsubscribe()
      history.disconnect()
      for (const event of ['focus', 'pagehide', 'pageshow']) window.removeEventListener(event, history.clear)
      document.removeEventListener('visibilitychange', history.clear)
    }
  }, [history])

  return <section className="page-width status-page training-history">
    <h1>我的提交</h1>
    <p>查看本人的训练提交。展开排队中或判题中的提交可自动更新状态；刷新第一页可查看新提交。</p>
    {account.status === 'authenticated' ? <>
      <div className="history-controls">
        {linked ? <button type="button" className="button button-neutral" disabled={account.busy || state.detail?.busy}
          onClick={() => void history.openLinkedDetail(id)}>读取本次提交</button> : null}
        <button type="button" className="button button-outline" disabled={state.busy || account.busy} onClick={() => void history.first()}>
          {state.page ? '刷新第一页' : '查看提交历史'}
        </button>
        <button type="button" className="button button-outline" disabled={state.busy || account.busy || !state.page?.next_cursor}
          onClick={() => void history.next()}>查看下一页</button>
      </div>
      <p role="status">{state.notice}</p>
      {linked && !state.page && state.detail?.id === id ? state.detail.value ?
        <ol className="history-list" aria-label="本次提交"><HistoryRow row={state.detail.value} history={history}
          detail={state.detail} accountBusy={account.busy} linked /></ol> :
        <p role="status">{state.detail.notice}</p> : null}
      {state.page?.submissions.length ? <ol className="history-list" aria-label="训练提交记录">
        {state.page.submissions.map(row => <HistoryRow key={row.id} row={row} history={history}
          detail={state.detail?.id === row.id ? state.detail : null} accountBusy={account.busy} />)}
      </ol> : null}
    </> : <div className="empty-state">
      <h2>{account.status === 'anonymous' ? '登录后查看提交历史' : '当前会话未确认'}</h2>
      {account.status === 'checking' || account.busy ? <p role="status">正在检查当前会话…</p> : account.status === 'anonymous' ?
        <Link className="button button-neutral" to="/login">登录已有账号</Link> :
        <button type="button" className="button button-outline" onClick={() => void accountController.run('check')}>重新检查会话</button>}
      <Link className="text-link" to="/">返回题库预览</Link>
    </div>}
  </section>
}
