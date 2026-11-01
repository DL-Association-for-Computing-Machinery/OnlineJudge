import { useEffect, useRef } from 'react'
import { minimalSetup } from 'codemirror'
import { EditorState } from '@codemirror/state'
import { EditorView, highlightActiveLine, lineNumbers } from '@codemirror/view'
import { cpp } from '@codemirror/lang-cpp'

export function CodeEditor({ value, onChange }: { value: string; onChange: (code: string) => void }) {
  const host = useRef<HTMLDivElement>(null)
  const view = useRef<EditorView | null>(null)
  const applying = useRef(false)
  useEffect(() => {
    if (!host.current) return
    const instance = new EditorView({
      parent: host.current,
      state: EditorState.create({ extensions: [
        minimalSetup, lineNumbers(), highlightActiveLine(), cpp(), EditorState.tabSize.of(4),
        EditorView.contentAttributes.of({ 'aria-label': '代码编辑器', spellcheck: 'false' }),
        EditorView.updateListener.of(update => {
          if (update.docChanged && !applying.current) onChange(update.state.doc.toString())
        }),
        EditorView.theme({
          '&': { height: '100%', backgroundColor: '#fff' },
          '.cm-scroller': { overflow: 'auto', fontFamily: 'ui-monospace, SFMono-Regular, Consolas, monospace', fontSize: '14px', lineHeight: '1.8' },
          '.cm-content': { padding: '16px 0', caretColor: '#c4510a' },
          '.cm-gutters': { backgroundColor: '#fafbfc', border: 'none', color: '#667085', padding: '0 8px' },
          '.cm-activeLine, .cm-activeLineGutter': { backgroundColor: '#faf8f4' },
          '&.cm-focused': { outline: '2px solid #c4510a', outlineOffset: '-2px' },
        }),
      ] }),
    })
    view.current = instance
    return () => { instance.destroy(); view.current = null }
  }, [onChange])
  useEffect(() => {
    const instance = view.current
    if (!instance || instance.state.doc.toString() === value) return
    applying.current = true
    try { instance.dispatch({ changes: { from: 0, to: instance.state.doc.length, insert: value } }) }
    finally { applying.current = false }
  }, [value])
  return <div className="code-editor" ref={host} />
}
