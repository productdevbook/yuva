import { Trans } from "@lingui/react/macro"
import { useSyncExternalStore } from "react"

import { cn } from "@/lib/utils"

type Toast = { id: number; text: React.ReactNode; undo?: () => void }

let current: Toast | null = null
let timer: ReturnType<typeof setTimeout> | undefined
let seq = 0
const listeners = new Set<() => void>()

function set(next: Toast | null) {
  current = next
  listeners.forEach((l) => l())
}

export function toast(text: React.ReactNode, undo?: () => void) {
  clearTimeout(timer)
  const t = { id: ++seq, text, undo }
  set(t)
  timer = setTimeout(() => current?.id === t.id && set(null), undo ? 6000 : 2200)
}

export function Toaster() {
  const t = useSyncExternalStore(
    (l) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
    () => current,
  )
  return (
    <div
      role="status"
      aria-live="polite"
      className={cn(
        "fixed bottom-6 left-1/2 z-[60] flex max-w-[calc(100%-2rem)] -translate-x-1/2 items-center gap-3 rounded-full bg-foreground px-4 py-2 text-[13px] text-background transition-opacity duration-200",
        t ? "opacity-100" : "pointer-events-none opacity-0",
      )}
      data-testid="toast"
    >
      <span className="min-w-0 truncate">{t?.text}</span>
      {t?.undo && (
        <button
          type="button"
          className="shrink-0 font-semibold underline underline-offset-3"
          onClick={() => {
            const undo = t.undo
            set(null)
            undo?.()
          }}
          data-testid="toast-undo"
        >
          <Trans>Undo</Trans>
        </button>
      )}
    </div>
  )
}
