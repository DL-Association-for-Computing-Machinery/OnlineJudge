import { createContext, useContext } from 'react'

export const PreviewDraftContext = createContext<{
  documents: Readonly<Record<string, string>>
  updateDocument: (id: string, code: string) => void
} | null>(null)

export function usePreviewDrafts() {
  const value = useContext(PreviewDraftContext)
  if (!value) throw new Error('预览编辑器缺少临时代码作用域')
  return value
}
