import { useState } from 'react'
import type { SessionDevice } from '../services/session'

const date = new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'medium' })

export function PendingDeviceChoice({ devices, disabled, replace }: {
  devices: SessionDevice[]; disabled: boolean; replace: (id: string) => void
}) {
  const [selected, setSelected] = useState<string | null>(null)
  return <fieldset disabled={disabled}>
    <legend>选择要退出的旧会话</legend>
    {devices.map((device, index) => <p key={device.id}>
      <label>
        <input type="radio" name="pending-device" checked={selected === device.id} onChange={() => setSelected(device.id)} />
        {' 会话 ' + String(index + 1) + ' · 登录于 ' + date.format(new Date(device.created_at)) +
          ' · 空闲到期 ' + date.format(new Date(device.idle_expires_at))}
      </label>
    </p>)}
    <button type="button" disabled={!selected} onClick={() => { if (selected) replace(selected) }}>退出所选会话并登录</button>
  </fieldset>
}
