import { useEffect, useRef } from "react"

export type CommandName = "reply" | "note" | "snooze" | "hand" | "labels" | "close" | "contact" | "more" | "history"

type Handlers = Partial<Record<CommandName, () => void>>

let active: { current: Handlers } | null = null

export function useCommandHandlers(handlers: Handlers) {
  const ref = useRef(handlers)
  ref.current = handlers
  useEffect(() => {
    active = ref
    return () => {
      if (active === ref) active = null
    }
  }, [])
}

export function runCommand(name: CommandName) {
  active?.current[name]?.()
}

export function hasCommands() {
  return active !== null
}
