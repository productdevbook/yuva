import { useEffect } from "react"

import { useLatest } from "@/hooks/use-latest"

export type CommandName =
  | "reply"
  | "note"
  | "snooze"
  | "hand"
  | "assignTeam"
  | "labels"
  | "progress"
  | "close"
  | "contact"
  | "more"
  | "history"
  | "discussion"
  | "incident"
  | "article"
  | "idea"
  | "createIssue"
  | "linkIssue"
  | "trash"
  | "restore"

type Handlers = Partial<Record<CommandName, () => void>>

let active: { current: Handlers } | null = null

export function useCommandHandlers(handlers: Handlers) {
  const ref = useLatest(handlers)
  useEffect(() => {
    active = ref
    return () => {
      if (active === ref) active = null
    }
  }, [ref])
}

export function runCommand(name: CommandName) {
  active?.current[name]?.()
}

export function hasCommands() {
  return active !== null
}
