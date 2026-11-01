import { createContext, useContext } from 'react'
import type { AccountController, AccountState } from './controller'

export const AccountContext = createContext<{ state: AccountState; controller: AccountController } | null>(null)

export function useCurrentAccount() {
  const value = useContext(AccountContext)
  if (!value) throw new Error('账号组件必须位于 AccountProvider 内')
  return value
}
