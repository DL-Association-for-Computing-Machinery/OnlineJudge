import { useEffect, useState, useSyncExternalStore } from 'react'
import { useBeforeUnload } from 'react-router'
import { useCurrentAccount } from '../account/context'
import { subscribePrivateInvalidation } from '../account/privateInvalidation'
import { AccountError, accountRequest } from '../services/session'
import { CodeDraftSession } from './codeDraftSession'

export function useCodeDraftSession(owner: string, problem: string, starter: string) {
  const { controller } = useCurrentAccount()
  const [gate] = useState(() => {
    let invalidated = false, verified = -1
    return { invalidated: () => invalidated, revoke: () => { invalidated = true },
      generation: () => verified, verify: (generation: number) => { verified = generation } }
  })
  const [verifying, setVerifying] = useState(true)
  const [verifyFailed, setVerifyFailed] = useState(false)
  const [draft] = useState(() => new CodeDraftSession([owner, 'preview-personal', problem, 'gnu++17'], starter, () => {
    const { status, session } = controller.getSnapshot()
    return !gate.invalidated() && gate.generation() === controller.readPrivateGeneration() &&
      status === 'authenticated' && session?.subject_id === owner &&
      Math.min(Date.parse(session.idle_expires_at), Date.parse(session.absolute_deadline)) > Date.now()
  }))
  const state = useSyncExternalStore(draft.subscribe, draft.getSnapshot)
  useEffect(() => {
    let timer: ReturnType<typeof setTimeout>
    let request: AbortController | undefined
    let live = true
    let started = false
    let recheck = false
    const revoke = () => {
      if (gate.invalidated()) return
      gate.revoke()
      request?.abort()
      draft.hide()
      setVerifying(false)
      setVerifyFailed(false)
    }
    const check = () => {
      clearTimeout(timer)
      const { status, session } = controller.getSnapshot()
      if (status !== 'authenticated' || session?.subject_id !== owner) { revoke(); return }
      const until = Math.min(Date.parse(session.idle_expires_at), Date.parse(session.absolute_deadline))
      if (!(until > Date.now())) { revoke(); return }
      timer = setTimeout(check, Math.min(until - Date.now(), 2_147_483_647))
    }
    const release = subscribePrivateInvalidation(revoke)
    const account = controller.subscribe(check)
    const restorePage = (event: PageTransitionEvent) => { if (event.persisted) revoke() }
    const verify = () => {
      if (gate.invalidated() || (started && !navigator.onLine)) return
      gate.verify(-1)
      setVerifying(true)
      setVerifyFailed(false)
      request?.abort()
      const pending = new AbortController()
      request = pending
      const generation = controller.readPrivateGeneration()
      const timeout = setTimeout(() => pending.abort(), 5_000)
      void accountRequest('read', pending.signal).then(session => {
        if (!live || request !== pending || gate.invalidated()) return
        if (generation !== controller.readPrivateGeneration() || session.subject_id !== owner ||
            Math.min(Date.parse(session.idle_expires_at), Date.parse(session.absolute_deadline)) <= Date.now()) {
          revoke()
          return
        }
        gate.verify(generation)
        setVerifying(false)
        if (!started) { started = true; draft.connect() }
        else draft.resume()
      }, (error: unknown) => {
        if (!live || request !== pending || gate.invalidated()) return
        if (error instanceof AccountError && error.code === 'SESSION_REQUIRED') { revoke(); return }
        setVerifyFailed(true)
      })
        .finally(() => clearTimeout(timeout))
    }
    const visible = () => {
      if (document.visibilityState === 'hidden') { recheck = true; return }
      if (recheck) { recheck = false; verify() }
    }
    const unfocused = () => { recheck = true }
    const focused = () => { if (recheck) { recheck = false; verify() } }
    window.addEventListener('pagehide', restorePage)
    window.addEventListener('blur', unfocused)
    window.addEventListener('focus', focused)
    document.addEventListener('visibilitychange', visible)
    check()
    verify()
    return () => {
      live = false; request?.abort(); clearTimeout(timer)
      window.removeEventListener('pagehide', restorePage)
      window.removeEventListener('blur', unfocused)
      window.removeEventListener('focus', focused)
      document.removeEventListener('visibilitychange', visible)
      release(); account(); draft.leave(!gate.invalidated())
    }
  }, [controller, draft, gate, owner])
  useBeforeUnload(event => {
    if (state.phase === 'dirty' || state.phase === 'saving' || state.phase === 'failed' || state.phase === 'conflict') {
      event.preventDefault()
      event.returnValue = true
    }
  })
  return { draft, state, verifying, verifyFailed }
}
