import { useLingui } from "@lingui/react/macro"
import { useMemo } from "react"
import { useLocation } from "react-router"

import type { ConversationFilters } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { useInboxes } from "@/lib/workspace"

export type Folder = "inbox" | "starred" | "snoozed" | "sent" | "archive" | "trash"

export const FOLDERS: Folder[] = ["inbox", "starred", "snoozed", "sent", "archive", "trash"]

const folderFilters: Record<Folder, ConversationFilters> = {
  inbox: { status: ["open"] },
  starred: { starred: true },
  snoozed: { status: ["snoozed"] },
  sent: { sent: "mine" },
  archive: { status: ["closed"] },
  trash: { trashed: true },
}

export function mailFilters(box: string, folder: Folder, q?: string): ConversationFilters {
  return {
    purpose: "mail",
    ...folderFilters[folder],
    inbox_id: box === "all" ? undefined : box,
    q: q || undefined,
  }
}

export function mailPath(box: string, folder: Folder, id?: string) {
  return `/mail/${box}/${folder}${id ? `/${id}` : ""}`
}

export function useMailInboxes() {
  const data = useInboxes().data
  return useMemo(() => (data ?? []).filter((i) => i.purpose === "mail"), [data])
}

export function useFolders(): Folder[] {
  const { canManage } = useSession()
  return canManage ? FOLDERS : FOLDERS.filter((f) => f !== "trash")
}

export function useFolderLabels(): Record<Folder, string> {
  const { t } = useLingui()
  return {
    inbox: t({ message: "Inbox", context: "mail folder" }),
    starred: t({ message: "Starred", context: "mail folder" }),
    snoozed: t({ message: "Snoozed", context: "mail folder" }),
    sent: t({ message: "Sent", context: "mail folder" }),
    archive: t({ message: "Archive", context: "mail folder" }),
    trash: t({ message: "Trash", context: "mail folder" }),
  }
}

export function useMailRoute() {
  const [, box = "all", folder, conversationId] =
    /^\/mail(?:\/([^/]+))?(?:\/([^/]+))?(?:\/([^/]+))?/.exec(useLocation().pathname) ?? []
  const inboxes = useMailInboxes()
  const known = box === "all" || inboxes.some((i) => i.id === box)
  const f = FOLDERS.includes(folder as Folder) ? (folder as Folder) : "inbox"
  return { box: known ? box : "all", folder: f, id: conversationId ?? null }
}
