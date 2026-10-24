import { useLingui } from "@lingui/react/macro"
import { useQueryClient } from "@tanstack/react-query"
import { useSyncExternalStore } from "react"

import { toast } from "@/components/common"
import { useErrorText } from "@/components/common/text"
import type { HelpPick } from "@/features/conversation/composer/ArticlePicker"
import { useIsChat, useIsLive } from "@/features/conversation/liveness"
import { macroSetsStatus, macroThen, type MacroPick } from "@/features/conversation/macros"
import { patchConversation, postMessage } from "@/features/conversation/queries"
import { api, unwrap, type Conversation, type ConversationUpdate, type Member, type Message } from "@/lib/api"
import { keys } from "@/lib/keys"
import { applyEvent } from "@/lib/live"
import { useSession } from "@/lib/session"
import { newId } from "@/lib/utils"
import { useLiveContext } from "@/lib/workspace"

export type ReplyDraft = {
  body: string
  mode: "message" | "note"
  files: File[]
  articles: HelpPick[]
  macro?: MacroPick
  cc?: string[]
  bcc?: string[]
}

const drafts = new Map<string, ReplyDraft>()
const revisions = new Map<string, number>()
const draftListeners = new Set<() => void>()

function subscribeDrafts(l: () => void) {
  draftListeners.add(l)
  return () => {
    draftListeners.delete(l)
  }
}

export function readDraft(id: string): ReplyDraft {
  return drafts.get(id) ?? { body: "", mode: "message", files: [], articles: [] }
}

export function writeDraft(id: string, d: ReplyDraft) {
  const before = drafts.get(id)?.body ?? ""
  if (d.body === "" && d.files.length === 0 && d.articles.length === 0 && !d.macro && d.mode === "message")
    drafts.delete(id)
  else drafts.set(id, d)
  if (before !== d.body) draftListeners.forEach((l) => l())
}

function restoreDraftOf(id: string, req: SendRequest) {
  const now = readDraft(id)
  writeDraft(id, {
    mode: "message",
    body: now.body || req.body,
    files: now.files.length ? now.files : req.files,
    articles: now.articles.length ? now.articles : (req.articles ?? []),
    macro: now.macro ?? req.macro,
    cc: now.cc ?? req.cc,
    bcc: now.bcc?.length ? now.bcc : req.bcc,
  })
  revisions.set(id, (revisions.get(id) ?? 0) + 1)
  draftListeners.forEach((l) => l())
}

export function useDraftText(id: string) {
  return useSyncExternalStore(subscribeDrafts, () =>
    drafts.get(id)?.mode === "message" ? (drafts.get(id)?.body.trim() ?? "") : ""
  )
}

export function useDraftRevision(id: string) {
  return useSyncExternalStore(subscribeDrafts, () => revisions.get(id) ?? 0)
}

export const UNDO_MS = 6000

export type PendingReply = {
  key: string
  body: string
  files: File[]
  close: boolean
  clientId: string
  messageId?: string
}

const pending = new Map<string, PendingReply[]>()
const pendingListeners = new Set<() => void>()
const NO_PENDING: PendingReply[] = []

function setPending(id: string, list: PendingReply[]) {
  if (list.length) pending.set(id, list)
  else pending.delete(id)
  pendingListeners.forEach((l) => l())
}

export function usePendingReplies(id: string) {
  return useSyncExternalStore(
    (l) => {
      pendingListeners.add(l)
      return () => {
        pendingListeners.delete(l)
      }
    },
    () => pending.get(id) ?? NO_PENDING
  )
}

const lines = new Map<string, Promise<unknown>>()

function inLine(id: string, run: () => Promise<void>) {
  const next = (lines.get(id) ?? Promise.resolve()).catch(() => undefined).then(run)
  lines.set(id, next)
  return next
}

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
    }, UNDO_MS)
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

export type SendRequest = {
  body: string
  files: File[]
  close: boolean
  suggestion?: Message
  articles?: HelpPick[]
  macro?: MacroPick
  cc?: string[]
  bcc?: string[]
  signature?: boolean
}

export function useConversationActions(c: Conversation, contactName: string) {
  const { t } = useLingui()
  const qc = useQueryClient()
  const ctx = useLiveContext()
  const errorText = useErrorText()
  const { membership } = useSession()
  const first = firstName(contactName)
  const chat = useIsChat(c)
  const live = useIsLive(c)
  const mail = c.inbox_purpose === "mail"

  const apply = (data: Conversation) => applyEvent(qc, ctx, { type: "conversation.updated", data })
  const patch = (body: ConversationUpdate) => patchConversation(c.id, body).then(apply)
  const failed = (err: unknown) => toast(errorText(err))
  const finish = (body: ConversationUpdate, done: string, revert: ConversationUpdate = revertOf(c)) =>
    patch(body).then(() => toast(done, () => patch(revert).catch(failed)), failed)

  const addMessage = (m: Message) => applyEvent(qc, ctx, { type: "message.created", data: m })

  const deliver = async (
    { body, files, suggestion, articles = [], macro, close, cc, bcc, signature }: SendRequest,
    clientId = newId(),
    onSent?: () => void
  ) => {
    let sent: Message
    if (suggestion && files.length === 0 && articles.length === 0 && !macro) {
      const path = { params: { path: { messageId: suggestion.id } } }
      if (body.trim() !== suggestion.body.trim())
        await unwrap(
          api.PATCH("/v1/messages/{messageId}", { ...path, body: suggestion.html ? { body, html: null } : { body } })
        )
      sent = await unwrap(
        api.POST("/v1/messages/{messageId}/send", { ...path, body: cc || bcc ? { cc, bcc } : undefined })
      )
    } else {
      sent = await postMessage(c.id, {
        kind: "message",
        body: body.trim() || undefined,
        client_id: clientId,
        files,
        article_ids: articles.length ? articles.map((a) => a.id) : undefined,
        macro_id: macro?.macro.id,
        cc,
        bcc,
        signature,
        // oxlint-disable-next-line unicorn/no-thenable -- the API names this field `then`
        then: macro ? macroThen(c, macro, close) : undefined,
      })
    }
    addMessage(sent)
    onSent?.()
    if (macro) {
      void qc.invalidateQueries({ queryKey: keys.conversation(ctx.ws, c.id) })
      void qc.invalidateQueries({ queryKey: keys.conversationLists(ctx.ws) })
    }
    if (suggestion && (files.length > 0 || articles.length > 0 || macro))
      await api
        .DELETE("/v1/messages/{messageId}", { params: { path: { messageId: suggestion.id } } })
        .catch(() => undefined)
  }
  const notSent = (req: SendRequest, err: unknown) => {
    restoreDraftOf(c.id, req)
    toast(t`Your reply to ${first} was not sent. ${errorText(err)}`)
  }

  return {
    chat,
    live,
    close: () =>
      finish(
        { status: "closed" },
        mail ? t`Archived` : c.kind === "feedback" ? t`Feedback resolved` : t`Conversation closed`
      ),
    reopen: () => finish({ status: "open" }, mail ? t`Moved to the Inbox` : t`Conversation reopened`),
    snooze: (at: Date, when: string) =>
      finish({ status: "snoozed", snooze_until: at.toISOString() }, t`${first} comes back ${when}`),
    untilReply: () => finish({ status: "pending" }, t`${first} comes back after writing again`),
    hand: (member: Member, note: string) => {
      const name = firstName(member.name || member.email)
      const noted = note.trim()
        ? postMessage(c.id, { kind: "note", body: note.trim(), client_id: newId(), files: [] }).then(addMessage)
        : Promise.resolve()
      noted
        .then(() => patch({ assignee_id: member.id }))
        .then(
          () => toast(t`Handed to ${name}`, () => patch({ assignee_id: c.assignee_id ?? null }).catch(failed)),
          failed
        )
    },
    claim: () =>
      patch({ assignee_id: membership.member_id }).then(
        () => toast(t`It is yours now`, () => patch({ assignee_id: c.assignee_id ?? null }).catch(failed)),
        failed
      ),
    note: (body: string, files: File[], mentions: string[] = []) =>
      postMessage(c.id, {
        kind: "note",
        body: body.trim() || undefined,
        client_id: newId(),
        files,
        mentions: mentions.length ? mentions : undefined,
      }).then(addMessage),
    send: (req: SendRequest): Promise<void> | undefined => {
      const { close } = req
      writeDraft(c.id, { body: "", mode: "message", files: [], articles: [] })
      const settled = !!req.macro && (close || macroSetsStatus(req.macro))
      if (live && !close) {
        return inLine(c.id, () => deliver(req)).catch((err) => {
          toast(t`Your reply to ${first} was not sent. ${errorText(err)}`)
          throw err
        })
      }
      if (live) {
        inLine(c.id, () => deliver(req)).then(
          () =>
            (req.macro ? Promise.resolve() : patch({ status: "closed" })).then(
              () =>
                toast(c.kind === "feedback" ? t`Sent to ${first} and resolved` : t`Sent to ${first} and closed`, () =>
                  patch(revertOf(c)).catch(failed)
                ),
              failed
            ),
          (err) => notSent(req, err)
        )
        return undefined
      }
      const key = `${c.id}:${newId()}`
      const status = close ? "closed" : mail ? c.status : "pending"
      const clientId = newId()
      const entry: PendingReply = {
        key,
        body: req.body,
        files: req.files,
        close,
        clientId,
        messageId:
          req.suggestion && req.files.length === 0 && !req.articles?.length && !req.macro
            ? req.suggestion.id
            : undefined,
      }
      const drop = () => {
        if (pending.get(c.id)?.some((p) => p.key === key))
          setPending(
            c.id,
            (pending.get(c.id) ?? []).filter((p) => p.key !== key)
          )
      }
      setPending(c.id, [...(pending.get(c.id) ?? []), entry])
      hold(key, () => {
        deliver(req, clientId, drop).then(
          () => {
            if (!settled && c.status !== status) patch({ status }).catch(failed)
          },
          (err) => {
            drop()
            notSent(req, err)
          }
        )
      })
      toast(
        close
          ? c.kind === "feedback"
            ? t`Sent to ${first} and resolved`
            : mail
              ? t`Sent to ${first} and archived`
              : t`Sent to ${first} and closed`
          : t`Sent to ${first}`,
        () => {
          if (cancel(key)) {
            drop()
            restoreDraftOf(c.id, req)
          } else {
            patch(revertOf(c)).catch(failed)
          }
        }
      )
      return undefined
    },
  }
}

export type ConversationActions = ReturnType<typeof useConversationActions>
