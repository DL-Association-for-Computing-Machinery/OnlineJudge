const channelName = 'oj-private-content-v1'
const localEvent = 'oj-private-content-invalidated'

// Notifications only revoke local display permission. They never establish an
// identity or authorize a reload, and contain no account or draft identifiers.
export function invalidatePrivateContent(): void {
  if (typeof window === 'undefined') return
  window.dispatchEvent(new Event(localEvent))
  if (typeof BroadcastChannel === 'undefined') return
  try {
    const channel = new BroadcastChannel(channelName)
    channel.postMessage('invalidate')
    channel.close()
  } catch { /* Focus/visibility revalidation remains available. */ }
}

export function subscribePrivateInvalidation(listener: () => void): () => void {
  window.addEventListener(localEvent, listener)
  let channel: BroadcastChannel | undefined
  try {
    if (typeof BroadcastChannel !== 'undefined') {
      channel = new BroadcastChannel(channelName)
      channel.onmessage = () => listener()
    }
  } catch { /* No private data is stored in a fallback persistence channel. */ }
  return () => { window.removeEventListener(localEvent, listener); channel?.close() }
}
