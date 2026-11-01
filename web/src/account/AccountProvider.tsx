import type { ReactNode } from 'react'
import { useAccount } from './useAccount'
import { AccountContext } from './context'

export function AccountProvider({ children }: { children: ReactNode }) {
  const account = useAccount()
  return <AccountContext.Provider value={account}>{children}</AccountContext.Provider>
}
