import assert from 'node:assert/strict'

// Exercise the real browser's IndexedDB engine, not a memory implementation.
// The standalone entry exists only in the driver's disposable asset directory.
export async function runJournalChecks(page, origin) {
  await page.goto(origin + '/author/drafts', { waitUntil: 'networkidle' })
  const evidence = await page.evaluate(async () => {
    const { ImportJournal } = await import('/journal-test/journal-harness.js')
    const a = new ImportJournal(), b = new ImportJournal()
    const owner = 'a'.repeat(32), other = 'b'.repeat(32), full = 'c'.repeat(32)
    const source = { source_id: 'library-checker:transaction', title: 'journal fixture', version: 'd'.repeat(64), import_revision: 'e'.repeat(64) }
    const check = (value, message) => { if (!value) throw new Error(message) }
    const failure = async action => { try { await action(); return false } catch { return true } }
    const first = await Promise.all([a.begin(owner, source), b.begin(owner, source)])
    check(first.filter(x => x.created).length === 1 && first[0].entry.key === first[1].entry.key, 'single insertion winner')
    const old = first[0].entry
    const receipt = { draft_id: 'f'.repeat(32), request_key: old.key, source_id: old.source, import_revision: old.revision }
    const confirmed = await a.confirm(old, receipt)
    const next = await b.begin(owner, { ...source, import_revision: '1'.repeat(64) }, confirmed)
    check(next.created && next.entry.key !== old.key && (await a.list(owner)).length === 2, 'new revision retains confirmed history')
    await a.confirm(old, receipt)
    await a.removeConfirmed(confirmed)
    check((await b.active(owner, source.source_id)).key === next.entry.key && (await b.list(owner)).length === 1, 'stale confirmation/delete preserves new pending')
    check(await failure(() => a.removeConfirmed(next.entry)), 'pending cannot be removed')
    check((await a.list(other)).length === 0, 'owner isolation')
    const capacity = await Promise.allSettled(Array.from({ length: 33 }, (_, i) => a.begin(full, { ...source, source_id: 'fixture:' + i })))
    check(capacity.filter(x => x.status === 'fulfilled').length === 32 && (await a.list(full)).length === 32, 'atomic capacity bound')
    const originalAdd = IDBObjectStore.prototype.add
    try {
      IDBObjectStore.prototype.add = function (...args) { if (this.name === 'records') throw new DOMException('fixture', 'QuotaExceededError'); return originalAdd.apply(this, args) }
      check(await failure(() => a.begin(other, source)), 'transaction write failure refuses import')
    } finally { IDBObjectStore.prototype.add = originalAdd }
    check((await a.list(other)).length === 0, 'failed transaction leaves no partial slot')
    const db = await new Promise((resolve, reject) => { const r = indexedDB.open('oj-draft-import-recovery', 1); r.onsuccess = () => resolve(r.result); r.onerror = () => reject(r.error) })
    const mutate = action => new Promise((resolve, reject) => { const tx = db.transaction(['records', 'slots'], 'readwrite'); action(tx); tx.oncomplete = resolve; tx.onerror = () => reject(tx.error); tx.onabort = () => reject(tx.error) })
    await mutate(tx => tx.objectStore('records').put({ ...next.entry, revision: 'corrupt' }, [owner, next.entry.key]))
    check(await failure(() => a.list(owner)), 'corruption refuses recovery')
    for (const damagedOwner of [undefined, other]) {
      await mutate(tx => tx.objectStore('records').put({ ...next.entry, owner: damagedOwner }, [owner, next.entry.key]))
      check(await failure(() => a.list(owner)), 'damaged owner cannot hide record')
      check(await failure(() => a.begin(owner, { ...source, source_id: 'fixture:new' })), 'damaged owner prevents allocation')
    }
    await mutate(tx => { tx.objectStore('records').clear(); tx.objectStore('slots').clear() })
    check(await failure(() => a.require(next.entry)), 'cleared storage cannot authorize memory retry')
    db.close(); a.close(); b.close()
    const c = new ImportJournal(); await c.list(other)
    const newer = await new Promise((resolve, reject) => { const r = indexedDB.open('oj-draft-import-recovery', 2); r.onsuccess = () => resolve(r.result); r.onerror = () => reject(r.error) })
    check(await failure(() => c.list(other)), 'versionchange closes and disables old connection')
    const d = new ImportJournal()
    check(await failure(() => d.list(other)) && newer.version === 2, 'unknown schema refused without destructive repair')
    newer.close(); c.close(); d.close()
    await new Promise((resolve, reject) => { const r = indexedDB.deleteDatabase('oj-draft-import-recovery'); r.onsuccess = resolve; r.onerror = () => reject(r.error) })
    return { firstWinners: 1, capacity: 32, durability: 'strict', cases: 13 }
  })
  assert.equal(evidence.capacity, 32)
  return evidence
}
