import { useLingui } from "@lingui/react/macro"
import { AlarmClockIcon, CheckCircle2Icon, CircleDashedIcon, CircleIcon } from "lucide-react"

import type { Conversation, ConversationStatus, ConversationUpdate, Priority } from "@/lib/api"

export type MenuName = "assign" | "status" | "priority" | "labels" | "move"

export type ControlProps = {
  conversation: Conversation
  update: (body: ConversationUpdate) => void
  openMenu: MenuName | null
  setOpenMenu: (m: MenuName | null) => void
}

export function menuProps(name: MenuName, p: Pick<ControlProps, "openMenu" | "setOpenMenu">) {
  return { open: p.openMenu === name, onOpenChange: (open: boolean) => p.setOpenMenu(open ? name : null) }
}


export const statusIcons: Record<ConversationStatus, React.ComponentType<{ className?: string }>> = {
  open: CircleIcon,
  pending: CircleDashedIcon,
  snoozed: AlarmClockIcon,
  closed: CheckCircle2Icon,
}

export const priorityClass: Record<Priority, string> = {
  urgent: "text-destructive",
  high: "text-brand",
  normal: "text-faint",
  low: "text-faint",
}

export function useSnoozeOptions() {
  const { t, i18n } = useLingui()
  const now = new Date()
  const inHour = new Date(now.getTime() + 60 * 60 * 1000)
  const tomorrow = new Date(now)
  tomorrow.setDate(now.getDate() + 1)
  tomorrow.setHours(9, 0, 0, 0)
  const nextWeek = new Date(now)
  nextWeek.setDate(now.getDate() + ((8 - now.getDay()) % 7 || 7))
  nextWeek.setHours(9, 0, 0, 0)
  const fmt = new Intl.DateTimeFormat(i18n.locale, { weekday: "short", hour: "2-digit", minute: "2-digit" })
  return [
    { at: inHour, label: t`In an hour`, when: fmt.format(inHour) },
    { at: tomorrow, label: t`Tomorrow morning`, when: fmt.format(tomorrow) },
    { at: nextWeek, label: t`Next week`, when: fmt.format(nextWeek) },
  ]
}
