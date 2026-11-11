import { useEffect, useState, useSyncExternalStore } from 'react'
import { useCurrentAccount } from '../account/context'
import { subscribePrivateInvalidation } from '../account/privateInvalidation'
import type { ReferenceReport } from '../services/referenceRequests'
import { ReferenceController } from './referenceController'
import './referenceRequests.css'

const labels = { queued: '待完成', cancelled: '已取消', reported: '已收到验证报告' }

export function ReferenceRequests({ draft }: { draft: string }) {
  const { controller: account, state: accountState } = useCurrentAccount()
  const [controller] = useState(() => new ReferenceController(account, draft))
  const { receipt, ready, report, busy, notice } = useSyncExternalStore(controller.subscribe, controller.getSnapshot)
  useEffect(() => {
    controller.connect()
    const unsubscribe = subscribePrivateInvalidation(controller.clear)
    return () => { unsubscribe(); controller.disconnect() }
  }, [controller])
  const disabled = busy || accountState.busy || accountState.status !== 'authenticated'
  return <section className="reference-requests" aria-labelledby="reference-request-title">
    <h2 id="reference-request-title">参考验证申请</h2>
    <p>为当前草稿版本提交验证申请。收到报告不代表验证通过，也不会发布题目。</p>
    <div className="reference-actions">
      <button className="button button-primary" type="button" disabled={disabled || receipt?.state === 'queued'} onClick={() => void controller.run('create')}>
        {receipt && receipt.state !== 'queued' ? '再次申请验证' : '提交或重试同一申请'}
      </button>
      <button className="button button-outline" type="button" disabled={disabled} onClick={() => void controller.run('read')}>查看申请回执</button>
      <button className="button button-outline" type="button" disabled={disabled || receipt?.state !== 'queued'} onClick={() => void controller.run('cancel')}>取消本次申请</button>
      {receipt?.state === 'reported' && <button className="button button-outline" type="button" disabled={disabled}
        onClick={() => void controller.run('report')}>查看验证结果</button>}
      {receipt?.state === 'queued' && <>
        <button className="button button-outline" type="button" disabled={disabled || ready === true} onClick={() => void controller.run('prepare')}>准备参考材料</button>
        <button className="button button-outline" type="button" disabled={disabled} onClick={() => void controller.run('plan')}>查看材料状态</button>
      </>}
    </div>
    <p role="status">{notice}{receipt ? ` 状态：${labels[receipt.state]}。` : ''}</p>
    {report && <ReferenceResult report={report} />}
  </section>
}

function ReferenceResult({ report }: { report: ReferenceReport }) {
  const { formal, comparison } = report
  const counterexample = comparison?.counterexample
  return <section className="reference-result" aria-labelledby="reference-result-title">
    <h3 id="reference-result-title">验证结果</h3>
    <dl>
      <dt>正式验证</dt>
      <dd>{formal.verdict} · 通过 {formal.passed} / {formal.total} 点</dd>
      <dt>独立对拍</dt>
      <dd>{comparison ? `${comparison.status === 'passed' ? '通过' : '发现差异'} · 一致 ${comparison.matched} / ${comparison.total} 点` : '这份报告没有对拍结果'}</dd>
    </dl>
    {counterexample && <details>
      <summary>查看第 {counterexample.case} 点的首个差异</summary>
      <dl>
        <dt>对拍输入</dt><dd><pre>{counterexample.input}</pre></dd>
        <dt>朴素解输出</dt><dd><pre>{counterexample.expected.join(' ')}</pre></dd>
        <dt>候选解输出</dt><dd><pre>{counterexample.actual.join(' ')}</pre></dd>
      </dl>
    </details>}
    <p>草稿版本 <code>{report.content_version}</code></p>
  </section>
}
