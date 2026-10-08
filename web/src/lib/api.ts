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
export type ApiKeyScope = S["ApiKeyScope"]
export type ApiKeyCreate = S["ApiKeyCreate"]
export type ApiKeyUpdate = S["ApiKeyUpdate"]
export type Passkey = S["Passkey"]
export type OAuthRequest = S["OAuthRequest"]
export type OAuthGrant = S["OAuthGrant"]
export type OAuthResource = S["OAuthResource"]
export type Inbox = S["Inbox"]
export type InboxUpdate = S["InboxUpdate"]
export type InboxMode = S["InboxMode"]
export type BusinessHours = S["BusinessHours"]
export type Weekday = S["Weekday"]
export type Channel = S["Channel"]
export type ChannelKind = S["ChannelKind"]
export type EmailChannel = S["EmailChannel"]
export type EmailChannelInput = S["EmailChannelInput"]
export type SmtpTls = S["SmtpTls"]
export type ChatChannel = S["ChatChannel"]
export type ChatChannelInput = S["ChatChannelInput"]
export type ChatLauncherPosition = S["ChatLauncherPosition"]
export type AppChannel = S["AppChannel"]
export type AppChannelInput = S["AppChannelInput"]
export type AppPlatform = S["AppPlatform"]
export type Availability = S["Availability"]
export type Typing = S["Typing"]
export type TypingAuthor = S["TypingAuthor"]
export type UndeliverableEmail = S["UndeliverableEmail"]
export type Contact = S["Contact"]
export type Conversation = S["Conversation"]
export type ConversationListItem = S["ConversationListItem"]
export type ConversationRead = S["ConversationRead"]
export type ConversationStatus = S["ConversationStatus"]
export type ConversationUpdate = S["ConversationUpdate"]
export type ConversationBulkUpdate = S["ConversationBulkUpdate"]
export type ConversationBulkFailure = S["ConversationBulkFailure"]
export type Feedback = S["Feedback"]
export type FeedbackCategory = S["FeedbackCategory"]
export type Priority = S["Priority"]
export type Message = S["Message"]
export type MessageAuthor = S["MessageAuthor"]
export type MessageDelivery = S["MessageDelivery"]
export type MessageCreate = S["MessageCreate"]
export type Attachment = S["Attachment"]
export type Label = S["Label"]
export type CannedReply = S["CannedReply"]
export type WebhookEndpoint = S["WebhookEndpoint"]
export type WebhookEventType = S["WebhookEventType"]
export type WebhookDeliveryState = S["WebhookDeliveryState"]
export type WebhookAttempt = S["WebhookAttempt"]
export type PushSubscriptionItem = S["PushSubscription"]
export type NotificationSettings = S["NotificationSettings"]
export type NotificationEvents = S["NotificationEvents"]
export type NotificationEventsUpdate = S["NotificationEventsUpdate"]
export type NotificationChannels = S["NotificationChannels"]
export type InboxNotifications = S["InboxNotifications"]
type Problem = S["Problem"]
export type RealtimeMessage = S["RealtimeMessage"]
export type ConversationQuery = NonNullable<paths["/v1/conversations"]["get"]["parameters"]["query"]>

export class ApiError extends Error {
  status: number
  code?: string
  detail?: string

  constructor(status: number, problem?: Partial<Problem>) {
    super(problem?.detail ?? problem?.title ?? `HTTP ${status}`)
    this.status = status
    this.code = problem?.code
    this.detail = problem?.detail
  }
}

export function isGone(err: unknown) {
  return err instanceof ApiError && (err.status === 404 || err.status === 403)
}

let workspaceId: string | null = null

export function setWorkspace(id: string | null) {
  workspaceId = id
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

export function attachmentUrl(a: Attachment, ws: string) {
  return `/v1/attachments/${encodeURIComponent(a.id)}?workspace_id=${encodeURIComponent(ws)}`
}

export function rawMessageUrl(messageId: string, ws: string) {
  return `/v1/messages/${encodeURIComponent(messageId)}/raw?workspace_id=${encodeURIComponent(ws)}`
}

export function useVersion() {
  return useQuery({
    queryKey: ["version"],
    queryFn: () => unwrap(api.GET("/v1/version")),
    staleTime: Infinity,
    retry: false,
  })
}
