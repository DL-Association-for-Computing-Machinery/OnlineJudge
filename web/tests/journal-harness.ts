// Compiled only by the isolated browser QA driver into its disposable assets.
// Production App routes never reference or ship this entry point.
export { ImportJournal, decodeEntry } from '../src/drafts/importJournal'
export { reserveReviewIntake, listReviewIntakes } from '../src/drafts/reviewIntakeJournal'

import { openDB } from 'idb'
export async function seedReportedReferenceKey(owner: string, draft: string, key: string) {
  const db = await openDB('oj-reference-request-v1', 1, { upgrade: db => { db.createObjectStore('active') } })
  try { await db.put('active', key, [owner, draft]) }
  finally { db.close() }
}
