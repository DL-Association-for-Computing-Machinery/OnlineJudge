import { useEffect, useRef, useState } from 'react'
import { AccountError, sessionDevicesRequest } from '../services/session'
import type { SessionDevices, SessionView } from '../services/session'
import { subscribePrivateInvalidation } from './privateInvalidation'

const time = new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' })
type Selection = { id: string } | 'others'

export function SessionDevicesPanel({ session, disabled }: { session: SessionView; disabled: boolean }) {
  const [list, setList] = useState<SessionDevices | null>(null)
  const [selection, setSelection] = useState<Selection | null>(null)
  const [busy, setBusy] = useState(false)
  const [notice, setNotice] = useState('')
  const pending = useRef<AbortController | null>(null)

  useEffect(() => {
    const invalidate = () => {
      pending.current?.abort()
      pending.current = null
      setBusy(false)
      setList(null)
      setSelection(null)
      setNotice('')
    }
    const unsubscribe = subscribePrivateInvalidation(invalidate)
    const visibility = () => { if (document.visibilityState === 'hidden') invalidate() }
    document.addEventListener('visibilitychange', visibility)
    window.addEventListener('pagehide', invalidate)
    return () => {
      pending.current?.abort()
      pending.current = null
      unsubscribe()
      document.removeEventListener('visibilitychange', visibility)
      window.removeEventListener('pagehide', invalidate)
    }
  }, [])

  const run = async (action: Selection | null) => {
    if (disabled || pending.current) return
    const current = list?.devices.find(device => device.current)
    if (action && !current) return
    const abort = new AbortController()
    pending.current = abort
    setBusy(true)
    setSelection(null)
    setList(null)
    setNotice(action ? '正在撤销会话…' : '正在读取会话…')
    try {
      if (action && current) {
        await sessionDevicesRequest(abort.signal, { current: current.id, target: action === 'others' ? undefined : action.id })
        if (!abort.signal.aborted) setNotice('已撤销所选会话。请重新读取列表确认当前状态。')
      } else {
        const result = await sessionDevicesRequest(abort.signal)
        if (abort.signal.aborted) return
        const observed = result.devices.find(device => device.current)
        if (result.subject_id !== session.subject_id || observed?.created_at !== session.created_at ||
            observed.absolute_deadline !== session.absolute_deadline) {
          setNotice('当前会话已变化。请检查当前会话后重新读取。')
          return
        }
        setList(result)
        setNotice('已读取有效会话。')
      }
    } catch (error) {
      if (abort.signal.aborted) return
      setNotice(error instanceof AccountError && error.kind === 'refused' ? error.message
        : action ? '撤销结果未确认。请重新读取列表核对，勿重复提交。' : '暂时无法读取会话，请手动重试。')
    } finally {
      if (pending.current === abort) {
        pending.current = null
        setBusy(false)
      }
    }
  }

  return (
    <section className="session-devices" aria-labelledby="session-devices-title" aria-busy={busy}>
      <h3 id="session-devices-title">已登录的会话</h3>
      <p>查看此账号仍有效的会话。撤销其他会话后，本次登录可以继续使用。</p>
      <div className="account-actions">
        <button type="button" className="secondary" disabled={disabled || busy} onClick={() => void run(null)}>读取会话列表</button>
        {list && list.devices.length > 1 && <button type="button" className="secondary" disabled={disabled || busy}
          onClick={() => setSelection('others')}>退出其他会话</button>}
      </div>
      {list && <ul className="session-device-list">
        {list.devices.map((device, index) => <li key={device.id}>
          <div>
            <strong>{device.current ? '当前会话' : '会话 ' + String(index + 1)}</strong>
            <p>登录于 {time.format(new Date(device.created_at))} · 空闲到期 {time.format(new Date(device.idle_expires_at))}</p>
          </div>
          {!device.current && <button type="button" className="secondary" disabled={disabled || busy}
            aria-label={'撤销会话 ' + String(index + 1)} onClick={() => setSelection({ id: device.id })}>撤销此会话</button>}
        </li>)}
      </ul>}
      {selection && <div className="account-actions">
        <p>{selection === 'others' ? '确认退出所有其他会话？' : '确认撤销所选会话？'}</p>
        <button type="button" disabled={disabled || busy} onClick={() => void run(selection)}>确认撤销</button>
        <button type="button" className="secondary" onClick={() => setSelection(null)}>取消</button>
      </div>}
      {notice && <p role="status">{notice}</p>}
    </section>
  )
}
