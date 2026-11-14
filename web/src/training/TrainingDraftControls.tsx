import { Link } from 'react-router'
import { downloadPreviewCode } from '../preview/codeFiles'
import type { useCodeDraftSession } from '../preview/useCodeDraftSession'

type DraftSession = ReturnType<typeof useCodeDraftSession>
const messages = {
  loading: '正在读取本地训练草稿…',
  ready: '代码会保存在此浏览器，供下次继续编辑。',
  dirty: '代码有尚未保存的更改。',
  saving: '正在保存本地训练草稿…',
  saved: '训练草稿已保存到此浏览器。',
  conflict: '另一个标签页已保存不同版本。请选择要保留的版本。',
  failed: '本地草稿未确认保存。可下载当前代码后重试。',
  cleared: '会话已变化。请检查账号后重新打开题目。',
}

export function TrainingDraftControls({ session, filename, locked }: { session: DraftSession; filename: string; locked: boolean }) {
  const { draft, state, verifying, verifyFailed } = session
  const unavailable = verifying || state.phase === 'cleared' || state.phase === 'loading'
  return <section aria-label="本地训练草稿">
    <p role="status">{verifyFailed ? '暂时无法核对会话，恢复连接后请重新打开题目。' :
      verifying ? '正在核对当前会话…' : messages[state.phase]}</p>
    {state.phase === 'cleared' ? <Link to="/account">检查账号状态</Link> : null}
    <div className="history-controls">
      {!unavailable && state.code ? <button type="button" className="button button-outline"
        onClick={() => downloadPreviewCode(filename, state.code, 'training')}>下载当前草稿</button> : null}
      {!unavailable && state.phase === 'failed' ? <button type="button" className="button button-outline"
        disabled={locked} onClick={() => draft.retry()}>重试本地保存</button> : null}
      {!unavailable && state.phase === 'conflict' ? <>
        <button type="button" className="button button-outline"
          disabled={locked} onClick={() => draft.chooseDraft(state.code)}>保留当前编辑</button>
        {state.stored !== undefined ? <>
          <button type="button" className="button button-outline"
            onClick={() => downloadPreviewCode(filename, state.stored ?? '', 'training')}>下载已存版本</button>
          <button type="button" className="button button-outline"
            disabled={locked} onClick={() => draft.chooseDraft(state.stored ?? '')}>保留已存版本</button>
        </> : null}
      </> : null}
    </div>
    {!unavailable ? state.alternatives.map((alternative, i) => <div className="history-controls" key={alternative.id}>
      <span>冲突副本 {i + 1}</span>
      <button type="button" className="button button-outline"
        onClick={() => downloadPreviewCode(filename, alternative.draft.code, 'training')}>下载副本</button>
      <button type="button" className="button button-outline" disabled={locked}
        onClick={() => {
          if (window.confirm('使用此副本会替换当前草稿。请先下载需要保留的版本。')) {
            draft.chooseDraft(alternative.draft.code, alternative.id)
          }
        }}>使用副本</button>
    </div>) : null}
  </section>
}
