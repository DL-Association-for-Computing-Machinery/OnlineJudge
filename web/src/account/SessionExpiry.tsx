import { useEffect, useState } from 'react'

export function SessionExpiry({ deadline }: { deadline: string }) {
  const [due, setDue] = useState(() => Date.now() >= Date.parse(deadline))
  useEffect(() => {
    const timer = setTimeout(() => setDue(true), Math.max(0, Date.parse(deadline) - Date.now()))
    return () => clearTimeout(timer)
  }, [deadline])
  return due ? <p className="account-notice uncertain" role="alert">按本机时间，会话可能已到期。请检查当前会话；页面不会自动续期。</p> : null
}
