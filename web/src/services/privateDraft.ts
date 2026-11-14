import { decodeStatement, validStatementID, type StatementPayload } from '../statement/payload'
import { readBoundedJSON, sameOriginJSONOptions } from '../transport/json'

export type DraftPreview = StatementPayload<'draft_id'>
export const validDraftID = validStatementID
const invalid = () => new Error('草稿预览暂不可用，请检查权限、服务状态或稍后手动重试。')

export function decodeDraftPreview(value: unknown, id: string): DraftPreview {
  return decodeStatement(value, id, 'draft_id', invalid)
}

export async function fetchDraftPreview(id: string, signal: AbortSignal): Promise<DraftPreview> {
  if (!validDraftID(id) || globalThis.location.protocol !== 'https:') throw invalid()
  try {
    const response = await fetch(`/api/v1/problem-drafts/${id}/preview`, {
      ...sameOriginJSONOptions(signal, 'same-origin'), method: 'GET',
    })
    return decodeDraftPreview(await readBoundedJSON(response, 7 * 1024 * 1024, invalid), id)
  } catch { throw invalid() }
}
