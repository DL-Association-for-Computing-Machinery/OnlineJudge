import { useState } from 'react'
import { validReturnReason } from '../services/reviewReturnReason'
import type { ReviewMaterialsController } from './materialsController'

export function ReviewApproval({ controller, state }: {
  controller: ReviewMaterialsController
  state: ReturnType<ReviewMaterialsController['getSnapshot']>
}) {
  const [reason, setReason] = useState('')
  const normalized = reason.trim()
  if (!state.eligibility?.approvalEnabled && state.approval !== 'pending' && state.approval !== 'recorded') return null
  return <section className="problem-statement" aria-labelledby="review-approval-title">
    <h2 id="review-approval-title">终审批准</h2>
    {state.approval === 'pending' && <button type="button" className="button button-outline" disabled={state.busy}
      onClick={() => void controller.approve()}>查询原批准回执</button>}
    {state.approval === 'recorded' && <p>已核实批准回执。题目发布另行处理。</p>}
    {state.approval === 'unavailable' && <p>本标签页无法保存或读取原请求记录，暂不能提交批准。</p>}
    {state.approval === 'ready' && state.eligibility?.eligible && state.returnStatus !== 'pending' && state.returnStatus !== 'recorded' &&
      <form className="private-draft-form" onSubmit={event => {
        event.preventDefault()
        if (!state.busy && validReturnReason(normalized)) {
          setReason('')
          void controller.approve(normalized)
        }
      }}>
        <label>批准理由
          <input value={reason} onChange={event => setReason(event.target.value)} maxLength={512}
            aria-describedby="review-approval-limit" autoComplete="off" required />
        </label>
        <p id="review-approval-limit">请填写单行理由，最多 512 字节。</p>
        <button className="button button-primary" type="submit" disabled={state.busy || !validReturnReason(normalized)}>
          确认批准
        </button>
      </form>}
  </section>
}
