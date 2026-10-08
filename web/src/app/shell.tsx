import { createContext, useContext } from "react"

export type Shell = {
  openDrawer: (q?: string) => void
  openPalette: () => void
  openShortcuts: () => void
}

export const ShellContext = createContext<Shell>({ openDrawer: () => {}, openPalette: () => {}, openShortcuts: () => {} })

export function useShell() {
  return useContext(ShellContext)
}
