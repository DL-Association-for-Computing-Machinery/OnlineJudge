import { openDB, type IDBPDatabase, type IDBPTransaction, type DBSchema } from 'idb'
import { object, validKey, validRevision, validSource, type ImportSource, type ImportTuple, type ImportReceipt } from '../services/draftManagement'
import { validDraftID } from '../services/privateDraft'

export type JournalEntry = ImportTuple & ({ status: 'pending' } | { status: 'confirmed'; draftId: string })
interface Slot { owner: string; source: string; key: string }
interface Schema extends DBSchema {
  records: { key: [string, string]; value: unknown }
  slots: { key: [string, string]; value: unknown }
}
export class JournalError extends Error {
  constructor() { super('本地恢复记录不可用、已满或已变化。未发起新的导入；请保留浏览器数据并重新检查。') }
}
export function decodeEntry(value: unknown): JournalEntry {
  if (!object(value) || typeof value.owner !== 'string' || !validDraftID(value.owner) || !validSource(value.source) || !validKey(value.key) || !validRevision(value.revision) ||
    !(value.status === 'pending' && Object.keys(value).length === 5 || value.status === 'confirmed' && Object.keys(value).length === 6 && typeof value.draftId === 'string' && validDraftID(value.draftId))) throw new JournalError()
  return value as unknown as JournalEntry
}
function decodeSlot(value: unknown): Slot {
  if (!object(value) || Object.keys(value).length !== 3 || typeof value.owner !== 'string' || !validDraftID(value.owner) || !validSource(value.source) || !validKey(value.key)) throw new JournalError()
  return value as unknown as Slot
}
export const sameTuple = (a: ImportTuple, b: ImportTuple) => a.owner === b.owner && a.source === b.source && a.key === b.key && a.revision === b.revision
const sameEntry = (a: JournalEntry, b: JournalEntry) => sameTuple(a, b) && a.status === b.status && (a.status === 'pending' || b.status === 'confirmed' && a.draftId === b.draftId)

async function readOwner(tx: IDBPTransaction<Schema, ['records', 'slots'], 'readonly' | 'readwrite'>, owner: string) {
  // Enumerate the immutable primary-key prefix, not an unvalidated value.owner
  // index: damaged owner fields must not hide a pending operation or bypass cap.
  const range = IDBKeyRange.bound([owner], [owner, []])
  const records: JournalEntry[] = []
  let cursor = await tx.objectStore('records').openCursor(range)
  while (cursor) {
    const record = decodeEntry(cursor.value)
    if (record.owner !== owner || JSON.stringify(cursor.primaryKey) !== JSON.stringify([owner, record.key]) || records.length >= 32) throw new JournalError()
    records.push(record); cursor = await cursor.continue()
  }
  let slot = await tx.objectStore('slots').openCursor(range)
  const active = new Set<string>()
  while (slot) {
    const value = decodeSlot(slot.value)
    if (active.size >= 32 || value.owner !== owner || JSON.stringify(slot.primaryKey) !== JSON.stringify([owner, value.source]) || !records.some(r => r.source === value.source && r.key === value.key)) throw new JournalError()
    active.add(value.key); slot = await slot.continue()
  }
  if (records.some(r => r.status === 'pending' && !active.has(r.key))) throw new JournalError()
  return records
}

export class ImportJournal {
  private opening: Promise<IDBPDatabase<Schema>> | null = null
  private unavailable = false
  private channel: BroadcastChannel | null = null
  connect(onChange: () => void) {
    try { this.channel = new BroadcastChannel('oj-import-journal-v1'); this.channel.onmessage = onChange } catch { /* IndexedDB transactions still arbitrate writers. */ }
  }
  close() { this.channel?.close(); this.channel = null; void this.opening?.then(db => db.close(), () => undefined); this.opening = null }
  private changed() { try { this.channel?.postMessage('invalidate') } catch { /* Notification is not write authority. */ } }
  private db() {
    if (this.unavailable) return Promise.reject(new JournalError())
    if (!this.opening) this.opening = new Promise<IDBPDatabase<Schema>>((resolve, reject) => {
      let abandoned = false
      const fail = () => { abandoned = true; this.unavailable = true; reject(new JournalError()) }
      const timeout = setTimeout(fail, 5_000)
      try {
        void openDB<Schema>('oj-draft-import-recovery', 1, {
          upgrade(db, oldVersion) {
            if (oldVersion !== 0) throw new JournalError()
            db.createObjectStore('records')
            db.createObjectStore('slots')
          },
          blocked: fail,
          blocking: () => { fail(); void this.opening?.then(db => db.close(), () => undefined) },
          terminated: fail,
        }).then(db => { clearTimeout(timeout); if (abandoned) db.close(); else resolve(db) }, () => { clearTimeout(timeout); fail() })
      } catch { clearTimeout(timeout); fail() }
    })
    return this.opening
  }
  async list(owner: string): Promise<JournalEntry[]> {
    if (!validDraftID(owner)) throw new JournalError()
    const db = await this.db()
    const tx = db.transaction(['records', 'slots'], 'readonly')
    try {
      const records = await readOwner(tx, owner)
      await tx.done
      return records
    } catch { try { tx.abort() } catch { /* Already completed. */ } await tx.done.catch(() => undefined); throw new JournalError() }
  }
  async begin(owner: string, source: ImportSource, verified?: JournalEntry): Promise<{ entry: JournalEntry; created: boolean }> {
    if (!validDraftID(owner) || !validSource(source.source_id) || !validRevision(source.import_revision)) throw new JournalError()
    const db = await this.db()
    const tx = db.transaction(['records', 'slots'], 'readwrite', { durability: 'strict' })
    try {
      if (tx.durability !== 'strict') throw new JournalError()
      const slots = tx.objectStore('slots'); const records = tx.objectStore('records')
      const existingRecords = await readOwner(tx, owner)
      const slotValue = await slots.get([owner, source.source_id])
      if (slotValue !== undefined) {
        const slot = decodeSlot(slotValue)
        if (slot.owner !== owner || slot.source !== source.source_id) throw new JournalError()
        const existing = decodeEntry(await records.get([owner, slot.key]))
        if (existing.owner !== owner || existing.source !== source.source_id || existing.key !== slot.key) throw new JournalError()
        if (existing.status === 'pending' || existing.revision === source.import_revision) { await tx.done; return { entry: existing, created: false } }
        if (!verified || verified.status !== 'confirmed' || !sameEntry(existing, verified)) throw new JournalError()
      }
      if (existingRecords.length >= 32) throw new JournalError()
      const entry: JournalEntry = { owner, source: source.source_id, key: crypto.randomUUID().replaceAll('-', ''), revision: source.import_revision, status: 'pending' }
      await records.add(entry, [owner, entry.key])
      await slots.put({ owner, source: entry.source, key: entry.key }, [owner, entry.source])
      // A successful put is insufficient: no POST may precede transaction commit.
      await tx.done; this.changed(); return { entry, created: true }
    } catch { try { tx.abort() } catch { /* Already completed. */ } await tx.done.catch(() => undefined); throw new JournalError() }
  }
  async active(owner: string, source: string): Promise<JournalEntry | undefined> {
    const db = await this.db(); const tx = db.transaction(['records', 'slots'], 'readonly')
    try {
      const value = await tx.objectStore('slots').get([owner, source])
      if (value === undefined) { await tx.done; return undefined }
      const slot = decodeSlot(value)
      if (slot.owner !== owner || slot.source !== source) throw new JournalError()
      const entry = decodeEntry(await tx.objectStore('records').get([owner, slot.key]))
      if (entry.owner !== owner || entry.source !== source || entry.key !== slot.key) throw new JournalError()
      await tx.done; return entry
    } catch { await tx.done.catch(() => undefined); throw new JournalError() }
  }
  async require(expected: JournalEntry) {
    const db = await this.db(); const tx = db.transaction('records', 'readonly')
    try {
      const current = decodeEntry(await tx.store.get([expected.owner, expected.key]))
      if (!sameEntry(current, expected)) throw new JournalError()
      await tx.done
    } catch { await tx.done.catch(() => undefined); throw new JournalError() }
  }
  async confirm(expected: JournalEntry, receipt: ImportReceipt): Promise<JournalEntry> {
    if (receipt.request_key !== expected.key || receipt.source_id !== expected.source || receipt.import_revision !== expected.revision || !validDraftID(receipt.draft_id)) throw new JournalError()
    const db = await this.db(); const tx = db.transaction('records', 'readwrite', { durability: 'strict' })
    try {
      if (tx.durability !== 'strict') throw new JournalError()
      const current = decodeEntry(await tx.store.get([expected.owner, expected.key]))
      if (!sameTuple(current, expected) || current.status === 'confirmed' && current.draftId !== receipt.draft_id) throw new JournalError()
      const confirmed: JournalEntry = { ...current, status: 'confirmed', draftId: receipt.draft_id }
      await tx.store.put(confirmed, [expected.owner, expected.key]); await tx.done; this.changed(); return confirmed
    } catch { try { tx.abort() } catch { /* Already completed. */ } await tx.done.catch(() => undefined); throw new JournalError() }
  }
  async removeConfirmed(expected: JournalEntry) {
    if (expected.status !== 'confirmed') throw new JournalError()
    const db = await this.db(); const tx = db.transaction(['records', 'slots'], 'readwrite', { durability: 'strict' })
    try {
      if (tx.durability !== 'strict') throw new JournalError()
      const records = tx.objectStore('records'); const slots = tx.objectStore('slots')
      const current = decodeEntry(await records.get([expected.owner, expected.key]))
      if (!sameEntry(current, expected)) throw new JournalError()
      const value = await slots.get([expected.owner, expected.source])
      if (value !== undefined) {
        const slot = decodeSlot(value)
        if (slot.owner !== expected.owner || slot.source !== expected.source) throw new JournalError()
        if (slot.key === expected.key) await slots.delete([expected.owner, expected.source])
      }
      await records.delete([expected.owner, expected.key]); await tx.done; this.changed()
    } catch { try { tx.abort() } catch { /* Already completed. */ } await tx.done.catch(() => undefined); throw new JournalError() }
  }
}
