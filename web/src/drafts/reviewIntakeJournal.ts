import { openDB, type DBSchema, type IDBPDatabase } from 'idb'
import { object, validKey, validRevision, type ReviewIntakeIntent } from '../services/draftManagement'
import { validDraftID } from '../services/privateDraft'

export interface SavedReviewIntake { intent: ReviewIntakeIntent; version: string }

interface ReviewJournalDB extends DBSchema {
  sent: { key: [string, string, string]; value: unknown }
}

function open(): Promise<IDBPDatabase<ReviewJournalDB>> {
  const pending = openDB<ReviewJournalDB>('oj-author-review-intake-v1', 1, {
    upgrade: db => { db.createObjectStore('sent') },
  })
  return new Promise((resolve, reject) => {
    let expired = false
    const timer = setTimeout(() => { expired = true; reject(new Error('Review journal unavailable')) }, 5000)
    void pending.then(db => {
      clearTimeout(timer)
      if (expired) db.close()
      else resolve(db)
    }, () => { clearTimeout(timer); reject(new Error('Review journal unavailable')) })
  })
}

// The key is derived from the immutable report ID. Strict storage records that
// this browser may have sent it, and atomically prevents a second tab writing.
export async function reserveReviewIntake(owner: string, entry: SavedReviewIntake): Promise<boolean> {
  const db = await open()
  const tx = db.transaction('sent', 'readwrite', { durability: 'strict' })
  try {
    if (tx.durability !== 'strict') throw new Error('Review journal unavailable')
    const { intent } = entry
    const key: [string, string, string] = [owner, intent.draft_id, intent.reference_request_id]
    const existing = await tx.store.get(key)
    if (existing === undefined) await tx.store.add(entry, key)
    await tx.done
    return existing === undefined
  } catch (err) {
    try { tx.abort() } catch { /* Already complete. */ }
    await tx.done.catch(() => undefined)
    throw err
  } finally { db.close() }
}

export async function hasReviewIntake(owner: string, intent: ReviewIntakeIntent): Promise<boolean> {
  const db = await open()
  try { return (await db.get('sent', [owner, intent.draft_id, intent.reference_request_id])) !== undefined }
  finally { db.close() }
}

export async function listReviewIntakes(owner: string, draft: string, after: string) {
  if (!validDraftID(owner) || !validDraftID(draft) || (after !== '' && !validDraftID(after))) throw new Error('Invalid review journal range')
  const db = await open()
  try {
    const rows = await db.getAll('sent', IDBKeyRange.bound([owner, draft, after], [owner, draft, '\uffff'], true), 21)
    return rows.map(row => {
      if (!object(row) || !validRevision(row.version) || !object(row.intent)) throw new Error('Invalid review journal record')
      const { request_key, draft_id, import_revision, reference_request_id } = row.intent
      if (draft_id !== draft || typeof reference_request_id !== 'string' || !validDraftID(reference_request_id) ||
        !validKey(request_key) || request_key !== `web-review-v1-${reference_request_id}` || !validRevision(import_revision)) {
        throw new Error('Invalid review journal intent')
      }
      return { version: row.version, intent: { request_key, draft_id, import_revision, reference_request_id } }
    })
  } finally { db.close() }
}
