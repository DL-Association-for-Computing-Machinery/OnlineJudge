import { useEffect, useState, useSyncExternalStore } from 'react'
import { AccountController } from './controller'
import { subscribePrivateInvalidation } from './privateInvalidation'

export function useAccount() {
  const [controller] = useState(() => new AccountController())
  const state = useSyncExternalStore(controller.subscribe, controller.getSnapshot)
  useEffect(() => {
    const release = subscribePrivateInvalidation(controller.revokePrivateGeneration)
    controller.connect()
    return () => { release(); controller.revokePrivateGeneration(); controller.disconnect() }
  }, [controller])
  return { state, controller }
}
