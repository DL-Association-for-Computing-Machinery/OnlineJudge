import { useState } from 'react'
import { Icon } from '../ui/Icon'

export function Sample({ label, text }: { label: string; text: string }) {
  const [notice, setNotice] = useState('')
  async function copy() {
    try { await navigator.clipboard.writeText(text); setNotice('已复制') }
    catch { setNotice('无法复制，请手动选择样例。') }
  }
  return <div className="sample"><div className="sample-heading"><h3>{label}</h3><button type="button" className="text-button" aria-label={`复制样例${label}`} onClick={() => void copy()}><Icon name={notice === '已复制' ? 'check' : 'copy'} />{notice === '已复制' ? notice : '复制'}</button></div><pre>{text}</pre><span className="sr-only" role="status">{notice}</span></div>
}
