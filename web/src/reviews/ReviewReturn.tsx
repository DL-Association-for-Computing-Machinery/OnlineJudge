import { useState } from 'react'
import { validReturnReason } from '../services/reviewReturnReason'
import type { ReviewMaterialsController } from './materialsController'

export function ReviewReturn({ controller, state }: {
  controller: ReviewMaterialsController
  state: ReturnType<ReviewMaterialsController['getSnapshot']>
}) {
  const [reason, setReason] = useState('')
  const normalized = reason.trim()
  const alreadyReturned = state.eligibility?.blockers.includes('materials_returned')
  return <section className="problem-statement" aria-labelledby="review-return-title">
    <h2 id="review-return-title">退回材料</h2>
    {state.returnStatus === 'pending' && <>
      <p>请保留本标签页并查询原退回回执；暂时查不到并不表示退回失败。</p>
      <button type="button" className="button button-outline" disabled={state.busy}
        onClick={() => void controller.returnReview()}>查询原退回回执</button>
    </>}
    {state.returnStatus === 'ready' && !alreadyReturned && <details>
      <summary>填写退回理由</summary>
      <p>作者能看到理由。退回后，作者须更新材料或验证证据才能重新送审。</p>
      <form className="private-draft-form" onSubmit={event => {
        event.preventDefault()
        if (!state.busy && validReturnReason(normalized)) {
          setReason('')
          void controller.returnReview(normalized)
        }
      }}>
        <label>退回理由
          <input value={reason} onChange={event => setReason(event.target.value)} maxLength={512}
            aria-describedby="review-return-limit" autoComplete="off" required />
        </label>
        <p id="review-return-limit">请填写单行理由，最多 512 字节。</p>
        <button className="button button-primary" type="submit" disabled={state.busy || !validReturnReason(normalized)}>
          确认退回材料
        </button>
      </form>
    </details>}
    {state.returnStatus === 'recorded' && <p>作者可以用原送审记录查看退回理由。</p>}
    {state.returnStatus !== 'recorded' && alreadyReturned && <p>这份材料已有退回决定。</p>}
    {state.returnStatus === 'unavailable' && !alreadyReturned && <p>本标签页无法保存或读取原请求记录，暂不能提交退回。</p>}
  </section>
}
