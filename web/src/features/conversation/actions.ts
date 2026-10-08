import { useLingui } from "@lingui/react/macro"
import { useQueryClient } from "@tanstack/react-query"

import { toast } from "@/components/common"
import { useErrorText } from "@/components/common/text"
import { patchConversation, postMessage } from "@/features/conversation/queries"
import { useQueue } from "@/features/inbox/queue"
import { api, unwrap, type Conversation, type ConversationUpdate, type Member, type Message } from "@/lib/api"
import { applyEvent } from "@/lib/live"
import { useSession } from "@/lib/session"
import { useLiveContext } from "@/lib/workspace"

export type ReplyDraft = { body: string; mode: "message" | "note"; files: File[] }

const drafts = new Map<string, ReplyDraft>()

export function readDraft(id: string): ReplyDraft {
  return drafts.get(id) ?? { body: "", mode: "message", files: [] }
}

export function writeDraft(id: string, d: ReplyDraft) {
  if (d.body === "" && d.files.length === 0 && d.mode === "message") drafts.delete(id)
  else drafts.set(id, d)
}

export const UNDO_MS = 6000

const outbox = new Map<string, ReturnType<typeof setTimeout>>()
const runners = new Map<string, () => void>()

function hold(key: string, run: () => void) {
  runners.set(key, run)
  outbox.set(
    key,
    setTimeout(() => {
      outbox.delete(key)
      runners.delete(key)
      run()
    }, UNDO_MS),
  )
}

function cancel(key: string) {
  const timer = outbox.get(key)
  if (timer === undefined) return false
  clearTimeout(timer)
  outbox.delete(key)
  runners.delete(key)
  return true
}

export function flushOutbox() {
  for (const [key, run] of runners) {
    cancel(key)
    run()
  }
}

window.addEventListener("beforeunload", (e) => {
  if (outbox.size === 0) return
  flushOutbox()
  e.preventDefault()
})

export function firstName(name: string) {
  return name.trim().split(/\s+/)[0] || name
}

function revertOf(c: Conversation): ConversationUpdate {
  if (c.status === "snoozed" && c.snooze_until && new Date(c.snooze_until) > new Date()) {
    return { status: "snoozed", snooze_until: c.snooze_until }
  }
  return { status: c.status }
}

export type SendRequest = { body: string; files: File[]; close: boolean; suggestion?: Message }

export function useQueueActions(c: Conversation, contactName: string) {
  const { t } = useLingui()
  const qc = useQueryClient()
  const live = useLiveContext()
  const queue = useQueue()
  const errorText = useErrorText()
  const { membership } = useSession()
  const first = firstName(contactName)

  const apply = (data: Conversation) => applyEvent(qc, live, { type: "conversation.updated", data })
  const patch = (body: ConversationUpdate) => patchConversation(c.id, body).then(apply)
  const back = () => {
    queue.unleave(c.id)
    queue.show(c.id)
  }
  const failed = (err: unknown) => {
    queue.unleave(c.id)
    toast(errorText(err))
  }
  const finish = (body: ConversationUpdate, done: string, revert: ConversationUpdate = revertOf(c)) => {
    queue.leave(c.id)
    queue.advance(c.id)
    patch(body).then(
      () => toast(done, () => patch(revert).then(back, failed)),
      failed,
    )
  }

  const addMessage = (m: Message) => applyEvent(qc, live, { type: "message.created", data: m })

  return {
    close: () => finish({ status: "closed" }, t`Closed without a reply`),
    snooze: (at: Date, when: string) => finish({ status: "snoozed", snooze_until: at.toISOString() }, t`${first} comes back ${when}`),
    untilReply: () => finish({ status: "pending" }, t`${first} comes back after writing again`),
    later: () => {
      queue.defer(c.id)
      queue.advance(c.id)
      toast(t`${first} is at the end of the queue`)
    },
    hand: (member: Member, note: string) => {
      const name = firstName(member.name || member.email)
      queue.leave(c.id)
      queue.advance(c.id)
      const noted = note.trim()
        ? postMessage(c.id, { kind: "note", body: note.trim(), client_id: crypto.randomUUID(), files: [] }).then(addMessage)
        : Promise.resolve()
      noted
        .then(() => patch({ assignee_id: member.id }))
        .then(
          () => toast(t`Handed to ${name}`, () => patch({ assignee_id: c.assignee_id ?? null }).then(back, failed)),
          failed,
        )
    },
    claim: () =>
      patch({ assignee_id: membership.member_id }).then(
        () => toast(t`It is yours now`),
        (err) => toast(errorText(err)),
      ),
    note: (body: string, files: File[]) =>
      postMessage(c.id, { kind: "note", body: body.trim() || undefined, client_id: crypto.randomUUID(), files }).then(addMessage),
    send: ({ body, files, close, suggestion }: SendRequest) => {
      const key = `${c.id}:${crypto.randomUUID()}`
      const status = close ? "closed" : "pending"
      const restore = () => {
        const now = readDraft(c.id)
        writeDraft(c.id, { mode: "message", body: now.body || body, files: now.files.length ? now.files : files })
      }
      queue.leave(c.id)
      queue.advance(c.id)
      writeDraft(c.id, { body: "", mode: "message", files: [] })
      hold(key, () => {
        const deliver = async () => {
          let sent: Message
          if (suggestion && files.length === 0) {
            const path = { params: { path: { messageId: suggestion.id } } }
            if (body.trim() !== suggestion.body.trim()) await unwrap(api.PATCH("/v1/messages/{messageId}", { ...path, body: suggestion.html ? { body, html: null } : { body } }))
            sent = await unwrap(api.POST("/v1/messages/{messageId}/send", path))
          } else {
            sent = await postMessage(c.id, { kind: "message", body: body.trim() || undefined, client_id: crypto.randomUUID(), files })
            if (suggestion) await api.DELETE("/v1/messages/{messageId}", { params: { path: { messageId: suggestion.id } } }).catch(() => undefined)
          }
          addMessage(sent)
          if (c.status !== status) await patch({ status })
        }
        deliver().catch((err) => {
          restore()
          queue.unleave(c.id)
          toast(t`Your reply to ${first} was not sent. ${errorText(err)}`)
        })
      })
      toast(close ? t`Sent to ${first} and closed` : t`Sent to ${first}`, () => {
        if (cancel(key)) {
          restore()
          back()
        } else {
          patch(revertOf(c)).then(back, failed)
        }
      })
    },
  }
}

export type QueueActions = ReturnType<typeof useQueueActions>
