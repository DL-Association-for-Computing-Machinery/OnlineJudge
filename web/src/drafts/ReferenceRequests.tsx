import { useEffect, useState, useSyncExternalStore } from 'react'
import { useCurrentAccount } from '../account/context'
import { subscribePrivateInvalidation } from '../account/privateInvalidation'
import { ReferenceController } from './referenceController'
import './referenceRequests.css'

const labels = { queued: '待完成', cancelled: '已取消', reported: '已收到验证报告' }

export function ReferenceRequests({ draft }: { draft: string }) {
  const { controller: account, state: accountState } = useCurrentAccount()
  const [controller] = useState(() => new ReferenceController(account, draft))
  const { receipt, busy, notice } = useSyncExternalStore(controller.subscribe, controller.getSnapshot)
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
    </div>
    <p role="status">{notice}{receipt ? ` 状态：${labels[receipt.state]}。` : ''}</p>
  </section>
}
