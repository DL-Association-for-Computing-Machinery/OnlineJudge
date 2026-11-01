import { useCallback, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { useBeforeUnload } from 'react-router'
import { useCurrentAccount } from '../account/context'
import { PreviewDraftContext } from './draftContext'

function DraftScope({ children }: { children: ReactNode }) {
  const [documents, setDocuments] = useState<Record<string, string>>({})
  useBeforeUnload(useCallback(event => {
    if (Object.keys(documents).length > 0) event.preventDefault()
  }, [documents]))
  const updateDocument = useCallback((id: string, code: string) => {
    setDocuments(previous => ({ ...previous, [id]: code }))
  }, [])
  const value = useMemo(() => ({ documents, updateDocument }), [documents, updateDocument])
  return <PreviewDraftContext.Provider value={value}>{children}</PreviewDraftContext.Provider>
}

export function PreviewDraftProvider({ children }: { children: ReactNode }) {
  const { state } = useCurrentAccount()
  // Preview edits survive SPA back/forward, but never cross an observed identity
  // boundary. This is ephemeral memory, not the planned IndexedDB draft service.
  const owner = state.status === 'authenticated' && state.session ? state.session.subject_id :
    state.status === 'anonymous' || state.status === 'insecure' || state.status === 'unavailable' ? 'guest' : 'unverified'
  return <DraftScope key={owner}>{children}</DraftScope>
}
