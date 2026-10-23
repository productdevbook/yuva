import { useEffect } from "react"

import type { ReplyChoice } from "@/features/conversation/composer/CannedMenu"
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

type Insert = (choice: ReplyChoice) => void

let inserter: { current: Insert } | null = null

export function useInsertHandler(handler: Insert) {
  const ref = useLatest(handler)
  useEffect(() => {
    inserter = ref
    return () => {
      if (inserter === ref) inserter = null
    }
  }, [ref])
}

export function runInsert(choice: ReplyChoice) {
  inserter?.current(choice)
}

export function hasCommands() {
  return active !== null
}
