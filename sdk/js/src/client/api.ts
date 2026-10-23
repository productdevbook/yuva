import type { components } from "../schema.gen"

type Schemas = components["schemas"]
export type ClientSession = Schemas["ClientSession"]
export type ClientSessionInfo = Schemas["ClientSessionInfo"]
export type ClientInbox = Schemas["ClientInbox"]
export type ClientAttachmentLimits = Schemas["ClientAttachmentLimits"]
export type ClientContact = Schemas["ClientContact"]
export type ClientPresence = Schemas["ClientPresence"]
export type ClientConversation = Schemas["ClientConversation"]
export type ClientConversationPage = Schemas["ClientConversationPage"]
export type ClientConversationCreated = Schemas["ClientConversationCreated"]
export type ClientRatingCreate = Schemas["ClientRatingCreate"]
export type ClientFeedbackCreate = Schemas["ClientFeedbackCreate"]
export type PageRating = Schemas["PageRating"]
export type PageRatingCreate = Schemas["PageRatingCreate"]
export type ClientChatSettings = Schemas["ClientChatSettings"]
export type DiscussionMode = Schemas["DiscussionMode"]
export type DiscussionState = Schemas["DiscussionState"]
export type DiscussionSort = Schemas["DiscussionSort"]
export type DiscussionKind = Schemas["DiscussionKind"]
export type IdeaStage = Schemas["IdeaStage"]
export type RoadmapSuggestions = Schemas["RoadmapSuggestions"]
export type ClientInboxRoadmap = Schemas["ClientInboxRoadmap"]
export type ClientRoadmap = Schemas["ClientRoadmap"]
export type ClientRoadmapStages = Schemas["ClientRoadmapStages"]
export type ChangelogLabel = Schemas["ChangelogLabel"]
export type ClientInboxWhatsNew = Schemas["ClientInboxWhatsNew"]
export type ClientChangelog = Schemas["ClientChangelog"]
export type ClientWhatsNew = Schemas["ClientWhatsNew"]
export type ClientWhatsNewEntry = Schemas["ClientWhatsNewEntry"]
export type ClientWhatsNewImage = Schemas["ClientWhatsNewImage"]
export type DiscussionAuthor = Schemas["DiscussionAuthor"]
export type Discussion = Schemas["Discussion"]
export type DiscussionPost = Schemas["DiscussionPost"]
export type DiscussionDetail = Schemas["DiscussionDetail"]
export type DiscussionPage = Schemas["DiscussionPage"]
export type ClientDiscussionCreate = Schemas["ClientDiscussionCreate"]
export type DiscussionPostCreate = Schemas["DiscussionPostCreate"]
export type ClientDiscussionPostUpdate = Schemas["ClientDiscussionPostUpdate"]
export type ReaderToken = Schemas["ReaderToken"]
export type ReaderProfile = Schemas["ReaderProfile"]
export type Rating = Schemas["Rating"]
export type ClientMessage = Schemas["ClientMessage"]
export type MessageCard = Schemas["MessageCard"]
export type ClientInboxHelp = Schemas["ClientInboxHelp"]
export type ClientHelp = Schemas["ClientHelp"]
export type ClientHelpCenter = Schemas["ClientHelpCenter"]
export type ClientHelpCollection = Schemas["ClientHelpCollection"]
export type ClientHelpCollectionDetail = Schemas["ClientHelpCollectionDetail"]
export type ClientHelpArticle = Schemas["ClientHelpArticle"]
export type ClientHelpArticleSummary = Schemas["ClientHelpArticleSummary"]
export type ClientHelpSearchResult = Schemas["ClientHelpSearchResult"]
export type ClientHelpSearchResults = Schemas["ClientHelpSearchResults"]
export type ClientHelpReaction = Schemas["ClientHelpReaction"]
export type ClientHelpSearchClick = Schemas["ClientHelpSearchClick"]
export type HelpHighlight = Schemas["HelpHighlight"]
export type ClientMessagePage = Schemas["ClientMessagePage"]
export type ClientAttachment = Schemas["ClientAttachment"]
export type ClientReadState = Schemas["ClientReadState"]
export type ClientRealtimeMessage = Schemas["ClientRealtimeMessage"]
export type Problem = Schemas["Problem"]
export type ClientMailingList = Schemas["ClientMailingList"]
export type ClientMailingLists = Schemas["ClientMailingLists"]
export type ClientSubscriptionCreate = Schemas["ClientSubscriptionCreate"]
export type ClientSubscriptionAccepted = Schemas["ClientSubscriptionAccepted"]
export type ClientSubscription = Schemas["ClientSubscription"]
export type ClientSubscriptionList = Schemas["ClientSubscriptionList"]
export type ClientSubscriptionPut = Schemas["ClientSubscriptionPut"]
export type ClientSubscriptionStatus = Schemas["ClientSubscriptionStatus"]
export type SubscriptionState = Schemas["SubscriptionState"]
export type PublicStatusPage = Schemas["PublicStatusPage"]
export type PublicStatusComponent = Schemas["PublicStatusComponent"]
export type PublicStatusDay = Schemas["PublicStatusDay"]
export type StatusState = Schemas["StatusComponentStateValue"]

export type Fetch = (url: string, init: RequestInit) => Promise<Response>

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string
  ) {
    super(`${status} ${code}`)
  }
}

export interface Request {
  method?: string
  json?: unknown
  body?: Blob
  headers?: Record<string, string>
  token?: string | null
  blob?: boolean
}

export async function call<T>(
  fetcher: Fetch,
  server: string,
  path: string,
  { method = "GET", json, body, headers = {}, token, blob }: Request = {}
): Promise<T> {
  headers = { ...headers }
  if (token) headers.Authorization = `Bearer ${token}`
  let payload: BodyInit | undefined = body
  if (json !== undefined) {
    headers["Content-Type"] = "application/json"
    payload = JSON.stringify(json)
  }
  let response: Response
  try {
    response = await fetcher(server + path, { method, headers, body: payload, credentials: "omit" })
  } catch {
    throw new ApiError(0, "network")
  }
  if (!response.ok) {
    let code = "error"
    try {
      code = ((await response.json()) as Problem).code ?? code
    } catch {}
    throw new ApiError(response.status, code)
  }
  if (response.status === 204) return undefined as T
  return (blob ? await response.blob() : await response.json()) as T
}

export function mailingLists(fetcher: Fetch, server: string, channel: string): Promise<ClientMailingLists> {
  return call(fetcher, server, `/client/v1/channels/${encodeURIComponent(channel)}/lists`)
}

export function createSubscription(
  fetcher: Fetch,
  server: string,
  channel: string,
  input: ClientSubscriptionCreate
): Promise<ClientSubscriptionAccepted> {
  return call(fetcher, server, `/client/v1/channels/${encodeURIComponent(channel)}/subscriptions`, {
    method: "POST",
    json: input,
  })
}
