import type { Inbox, MailboxAccess } from "@/lib/api"

export function readsMailbox(inbox: Inbox) {
  return !inbox.mailbox || inbox.mailbox.access !== undefined
}

export function mailboxAccess(inbox: Inbox): MailboxAccess | undefined {
  return inbox.mailbox?.access
}

export function writesMailbox(inbox: Inbox | undefined) {
  const access = inbox?.mailbox?.access
  return !inbox?.mailbox || access === "owner" || access === "send"
}

export function managesInbox(inbox: Inbox, canManage: boolean) {
  return inbox.mailbox ? inbox.mailbox.access === "owner" : canManage
}
