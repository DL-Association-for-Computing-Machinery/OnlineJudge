import { openDB, type DBSchema, type IDBPDatabase } from 'idb'

export const codeDraftLimit = 128 * 1024
const ownerLimit = 32
const siteLimit = 128
export const codeDraftDBName = 'oj-local-code-drafts'

export type DraftScope = readonly [owner: string, context: string, problem: string, language: string]
type ScopeKey = [string, string, string, string, string]
export interface DraftVersion { code: string; revision: string }
interface DraftSchema extends DBSchema {
  epochs: { key: string; value: string }
  code: { key: ScopeKey; value: DraftVersion }
}
export interface LoadedDraft {
  epoch: string
  current?: DraftVersion
  conflicts: { id: string; draft: DraftVersion }[]
}
export type DraftWrite = { status: 'saved'; current: DraftVersion } | { status: 'conflict'; current?: DraftVersion } | { status: 'cleared' } | { status: 'paused' }
const key = (scope: DraftScope, slot = 'main'): ScopeKey => [...scope, slot]
const prefix = (parts: readonly string[]) => IDBKeyRange.bound([...parts], [...parts, []])
const baseline = 'initial'
const acceptable = (code: string) => code.length <= codeDraftLimit && !code.includes('\0') && new TextEncoder().encode(code).byteLength <= codeDraftLimit
const fail = () => new Error('Local code draft storage unavailable or full')

async function connection(): Promise<IDBPDatabase<DraftSchema>> {
  return new Promise((resolve, reject) => {
    let abandoned = false
    let active: IDBPDatabase<DraftSchema> | undefined
    const stop = () => { abandoned = true; clearTimeout(timer); active?.close(); reject(fail()) }
    const timer = setTimeout(stop, 4_000)
    try {
      void openDB<DraftSchema>(codeDraftDBName, 1, {
        upgrade(db) { db.createObjectStore('epochs'); db.createObjectStore('code') },
        blocked: stop,
        blocking: stop,
        terminated: stop,
      }).then(db => {
        clearTimeout(timer)
        if (abandoned) db.close()
        else { active = db; resolve(db) }
      }, stop)
    } catch { stop() }
  })
}

export async function loadCodeDraft(scope: DraftScope): Promise<LoadedDraft> {
  const db = await connection()
  try {
    const tx = db.transaction(['epochs', 'code'])
    const versions = tx.objectStore('code')
    const epoch = await tx.objectStore('epochs').get(scope[0]) ?? baseline
    const current = await versions.get(key(scope))
    const conflicts: LoadedDraft['conflicts'] = []
    for (let row = await versions.openCursor(prefix(scope)); row; row = await row.continue()) {
      const id = row.key[4]
      if (id !== 'main') conflicts.push({ id, draft: row.value })
    }
    await tx.done
    return { epoch, current, conflicts }
  } finally { db.close() }
}

export interface DraftChange {
  scope: DraftScope
  epoch: string
  expected?: string
  writer: string
  code: string
  discard?: { id: string; revision: string }
}

export async function writeCodeDraft(change: DraftChange, allowed: () => boolean): Promise<DraftWrite> {
  if (!acceptable(change.code)) throw fail()
  const db = await connection()
  try {
    const tx = db.transaction(['epochs', 'code'], 'readwrite', { durability: 'strict' })
    const versions = tx.objectStore('code')
    try {
      if (tx.durability !== 'strict') throw fail()
      const epoch = await tx.objectStore('epochs').get(change.scope[0]) ?? baseline
      if (epoch !== change.epoch) { await tx.done; return { status: 'cleared' } }
      if (!allowed()) { await tx.done; return { status: 'paused' } }
      const main = key(change.scope)
      const stored = await versions.get(main)
      const conflict = stored?.revision !== change.expected
      const destination = conflict ? key(change.scope, change.writer) : main
      const exists = await versions.getKey(destination)
      if (!exists && (await versions.count(prefix([change.scope[0]])) >= ownerLimit || await versions.count() >= siteLimit)) throw fail()
      if (!allowed()) { await tx.done; return { status: 'paused' } }
      const version = { code: change.code, revision: crypto.randomUUID() }
      await versions.put(version, destination)
      if (!conflict) {
        await versions.delete(key(change.scope, change.writer))
        if (change.discard && change.discard.id !== 'main') {
          const alternative = key(change.scope, change.discard.id)
          if ((await versions.get(alternative))?.revision === change.discard.revision) await versions.delete(alternative)
        }
      }
      await tx.done
      return conflict ? { status: 'conflict', current: stored } : { status: 'saved', current: version }
    } catch (error) {
      try { tx.abort() } catch { /* already ended */ }
      await tx.done.catch(() => undefined)
      throw error
    }
  } finally { db.close() }
}

export async function clearAccountCodeDrafts(owner: string): Promise<void> {
  const db = await connection()
  try {
    const tx = db.transaction(['epochs', 'code'], 'readwrite', { durability: 'strict' })
    try {
      if (tx.durability !== 'strict') throw fail()
      await tx.objectStore('epochs').put(crypto.randomUUID(), owner)
      await tx.objectStore('code').delete(prefix([owner]))
      await tx.done
    } catch (error) {
      try { tx.abort() } catch { /* already ended */ }
      await tx.done.catch(() => undefined)
      throw error
    }
  } finally { db.close() }
}
