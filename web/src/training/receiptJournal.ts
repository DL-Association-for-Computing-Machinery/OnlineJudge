export function trainingReceiptSlot(subject: string, problemID: string) {
  return ['oj.training-receipt.v1', subject, problemID].join(':')
}

export function readTrainingKey(slot: string) {
  const key = sessionStorage.getItem(slot)
  if (key !== null && !/^[a-f0-9]{8}(?:-[a-f0-9]{4}){3}-[a-f0-9]{12}$/.test(key)) throw new Error('Invalid training receipt key')
  return key
}

export function reserveTrainingKey(slot: string, key: string) {
  if (readTrainingKey(slot) !== null) return false
  sessionStorage.setItem(slot, key)
  return true
}

export function clearTrainingKey(slot: string, key: string) {
  const current = readTrainingKey(slot)
  if (current !== null && current !== key) return false
  if (current !== null) sessionStorage.removeItem(slot)
  return true
}
