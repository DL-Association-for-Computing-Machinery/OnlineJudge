import { openDB, type DBSchema, type IDBPDatabase } from 'idb'
import { validKey } from '../services/draftManagement'
import { validDraftID } from '../services/privateDraft'

interface Schema extends DBSchema {
  active: { key: [string, string]; value: unknown }
}
const unavailable = () => new Error('本地参考申请记录不可用，请保留浏览器数据。')
const readKey = (value: unknown) => {
  if (value !== undefined && !validKey(value)) throw unavailable()
  return value
}
function open(): Promise<IDBPDatabase<Schema>> {
  const request = openDB<Schema>('oj-reference-request-v1', 1, { upgrade: db => { db.createObjectStore('active') } })
  return new Promise((resolve, reject) => {
    let expired = false
    const timer = setTimeout(() => { expired = true; reject(unavailable()) }, 5000)
    void request.then(db => {
      clearTimeout(timer)
      if (expired) db.close()
      else resolve(db)
    }, () => { clearTimeout(timer); reject(unavailable()) })
  })
}

export interface ReferenceJournal {
  read(owner: string, draft: string): Promise<string | undefined>
  select(owner: string, draft: string, terminalKey?: string): Promise<string>
}
export const referenceJournal: ReferenceJournal = {
  async read(owner, draft) {
    if (!validDraftID(owner) || !validDraftID(draft)) throw unavailable()
    const db = await open()
    try { return readKey(await db.get('active', [owner, draft])) }
    finally { db.close() }
  },
  async select(owner, draft, terminalKey) {
    if (!validDraftID(owner) || !validDraftID(draft) || (terminalKey !== undefined && !validKey(terminalKey))) throw unavailable()
    const db = await open()
    const tx = db.transaction('active', 'readwrite', { durability: 'strict' })
    try {
      if (tx.durability !== 'strict') throw unavailable()
      const current = readKey(await tx.store.get([owner, draft]))
      if (terminalKey !== undefined && current === undefined) throw unavailable()
      // Two tabs can replace the same observed terminal key only once.
      if (current && current !== terminalKey) { await tx.done; return current }
      const key = crypto.randomUUID().replaceAll('-', '')
      await tx.store.put(key, [owner, draft])
      await tx.done
      return key
    } catch {
      try { tx.abort() } catch { /* Already completed. */ }
      await tx.done.catch(() => undefined)
      throw unavailable()
    } finally { db.close() }
  },
}
