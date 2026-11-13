import { useState } from 'react'
import type { ReviewMaterialsController } from './materialsController'
import './statementConfirmation.css'

const checks = ['题意与任务目标', '数据范围与约束', '输入和输出定义', '样例及其解释']

export function StatementConfirmation({ controller, state }: {
  controller: ReviewMaterialsController
  state: ReturnType<ReviewMaterialsController['getSnapshot']>
}) {
  const [checked, setChecked] = useState(() => checks.map(() => false))
  const allChecked = checked.every(Boolean)
  return <section className="problem-statement" aria-labelledby="statement-confirmation-title">
    <h2 id="statement-confirmation-title">题意确认</h2>
    {state.confirmation === 'ready' && <>
      <p>请根据上方冻结题面和样例逐项人工核对。确认只记录对这份题面的核对结果。</p>
      <fieldset className="statement-checklist">
        <legend>人工核对清单</legend>
        {checks.map((label, i) => <div key={label}><label>
          <input type="checkbox" checked={checked[i]} onChange={event => {
            const value = event.currentTarget.checked
            setChecked(previous => previous.map((item, j) => i === j ? value : item))
          }} /> {label}
        </label></div>)}
      </fieldset>
      <button type="button" className="button button-primary" disabled={!allChecked || state.busy}
        onClick={() => void controller.confirm('confirm', allChecked)}>记录题意确认</button>
    </>}
    {state.confirmation === 'pending' && <>
      <p role="status">{state.busy ? '正在核验题意确认回执…' : '尚未核实本次确认。请保留本标签页并查询原回执；暂时查不到并不表示写入失败。'}</p>
      <button type="button" className="button button-outline" disabled={state.busy}
        onClick={() => void controller.confirm('read')}>查询原确认回执</button>
    </>}
    {state.confirmation === 'recorded' && <p role="status">已核实这份冻结题面的确认回执。</p>}
    {state.confirmation === 'unavailable' && <p role="status">本标签页无法保存或读取原请求记录，暂不能记录题意确认。</p>}
  </section>
}
