import { useQuery } from "@tanstack/react-query"
import createClient from "openapi-fetch"

import type { components, paths } from "@/lib/schema.gen"

type S = components["schemas"]
export type Me = S["Me"]
export type Membership = S["Membership"]
export type Role = S["Role"]
export type Locale = S["Locale"]
export type Member = S["Member"]
export type Invite = S["Invite"]
export type ApiKey = S["ApiKey"]
export type Passkey = S["Passkey"]
export type Inbox = S["Inbox"]
export type InboxCreate = S["InboxCreate"]
export type InboxUpdate = S["InboxUpdate"]
export type InboxMode = S["InboxMode"]
export type BusinessHours = S["BusinessHours"]
export type Weekday = S["Weekday"]
export type Channel = S["Channel"]
export type ChannelKind = S["ChannelKind"]
export type Contact = S["Contact"]
export type Conversation = S["Conversation"]
export type ConversationStatus = S["ConversationStatus"]
export type ConversationUpdate = S["ConversationUpdate"]
export type Priority = S["Priority"]
export type Message = S["Message"]
export type MessageCreate = S["MessageCreate"]
export type Attachment = S["Attachment"]
export type Label = S["Label"]
export type CannedReply = S["CannedReply"]
export type UsageMonth = S["UsageMonth"]
export type Problem = S["Problem"]
export type RealtimeMessage = S["RealtimeMessage"]
export type ConversationQuery = NonNullable<paths["/v1/conversations"]["get"]["parameters"]["query"]>

export class ApiError extends Error {
  status: number
  code?: string

  constructor(status: number, problem?: Partial<Problem>) {
    super(problem?.detail ?? problem?.title ?? `HTTP ${status}`)
    this.status = status
    this.code = problem?.code
  }
}

let workspaceId: string | null = null

export function setWorkspace(id: string | null) {
  workspaceId = id
}

export function workspaceHeaders(): Record<string, string> {
  return workspaceId ? { "Yuva-Workspace": workspaceId } : {}
}

export const api = createClient<paths>({ baseUrl: window.location.origin, credentials: "same-origin" })

api.use({
  onRequest({ request }) {
    if (workspaceId && !request.headers.has("Yuva-Workspace")) {
      request.headers.set("Yuva-Workspace", workspaceId)
    }
    return request
  },
})

export async function unwrap<T>(call: Promise<{ data?: T; error?: unknown; response: Response }>): Promise<T> {
  const { data, error, response } = await call
  if (!response.ok) {
    throw new ApiError(response.status, (error ?? undefined) as Partial<Problem> | undefined)
  }
  return data as T
}

export async function downloadAttachment(a: Attachment) {
  const res = await fetch(`/v1/attachments/${a.id}`, { headers: workspaceHeaders(), credentials: "same-origin" })
  if (!res.ok) throw new ApiError(res.status)
  const url = URL.createObjectURL(await res.blob())
  const link = document.createElement("a")
  link.href = url
  link.download = a.filename
  document.body.append(link)
  link.click()
  link.remove()
  setTimeout(() => URL.revokeObjectURL(url), 10_000)
}

export function useVersion() {
  return useQuery({
    queryKey: ["version"],
    queryFn: () => unwrap(api.GET("/v1/version")),
    staleTime: Infinity,
    retry: false,
  })
}
