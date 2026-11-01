import { useEffect, useState, useSyncExternalStore } from 'react'
import { AccountController } from './controller'

export function useAccount() {
  const [controller] = useState(() => new AccountController())
  const state = useSyncExternalStore(controller.subscribe, controller.getSnapshot)
  useEffect(() => {
    controller.connect()
    return () => controller.disconnect()
  }, [controller])
  return { state, controller }
}
