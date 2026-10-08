import { useEffect, useRef } from "react"

export type CommandName = "reply" | "note" | "snooze" | "hand" | "close" | "contact" | "more" | "history" | "next"

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
