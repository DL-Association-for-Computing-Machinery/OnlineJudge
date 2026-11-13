import { useEffect, useState, useSyncExternalStore } from 'react'
import { useCurrentAccount } from '../account/context'
import { subscribePrivateInvalidation } from '../account/privateInvalidation'
import type { ReferenceReport } from '../services/referenceRequests'
import { ReviewIntakeController } from './reviewIntakeController'

export function ReviewIntake({ report }: { report: ReferenceReport & { import_revision: string } }) {
  const { controller: account } = useCurrentAccount()
  const [controller] = useState(() => new ReviewIntakeController(account, report))
  const { status, id } = useSyncExternalStore(controller.subscribe, controller.getSnapshot)
  useEffect(() => {
    controller.connect()
    const unsubscribe = subscribePrivateInvalidation(controller.clear)
    window.addEventListener('pagehide', controller.clear)
    document.addEventListener('visibilitychange', controller.clear)
    return () => {
      unsubscribe(); controller.disconnect()
      window.removeEventListener('pagehide', controller.clear)
      document.removeEventListener('visibilitychange', controller.clear)
    }
  }, [controller])

  return <section className="reference-requests" aria-labelledby="review-intake-title">
    <h3 id="review-intake-title">提交审核</h3>
    <p>把上方参考报告及其冻结草稿版本送审。收到送审 ID 后，请交给负责分配审核人的维护者。</p>
    {status === 'ready' && <button className="button button-primary" type="button"
      onClick={() => void controller.run('submit')}>送审此报告</button>}
    {status === 'busy' && <p role="status">正在处理送审记录…</p>}
    {status === 'pending' && <>
      <p role="status">请查询原送审回执。暂时查不到并不表示服务端没有收到请求。</p>
      <button className="button button-outline" type="button" onClick={() => void controller.run('read')}>查询原送审回执</button>
    </>}
    {status === 'recorded' && <p role="status">送审 ID：<code>{id}</code>。此回执仅表示材料已送审。</p>}
    {status === 'unavailable' && <p role="status">送审记录暂不可用。请重新打开草稿并查看当前参考报告。</p>}
  </section>
}
