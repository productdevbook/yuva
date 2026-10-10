import type { components } from "../schema.gen";

type Schemas = components["schemas"];
export type ClientSession = Schemas["ClientSession"];
export type ClientSessionInfo = Schemas["ClientSessionInfo"];
export type ClientInbox = Schemas["ClientInbox"];
export type ClientContact = Schemas["ClientContact"];
export type ClientPresence = Schemas["ClientPresence"];
export type ClientConversation = Schemas["ClientConversation"];
export type ClientConversationPage = Schemas["ClientConversationPage"];
export type ClientConversationCreated = Schemas["ClientConversationCreated"];
export type ClientRatingCreate = Schemas["ClientRatingCreate"];
export type ClientFeedbackCreate = Schemas["ClientFeedbackCreate"];
export type ClientQuestionCreate = Schemas["ClientQuestionCreate"];
export type PageRating = Schemas["PageRating"];
export type PageRatingCreate = Schemas["PageRatingCreate"];
export type ClientPageAnswer = Schemas["ClientPageAnswer"];
export type ClientPageAnswerList = Schemas["ClientPageAnswerList"];
export type Rating = Schemas["Rating"];
export type ClientMessage = Schemas["ClientMessage"];
export type ClientMessagePage = Schemas["ClientMessagePage"];
export type ClientAttachment = Schemas["ClientAttachment"];
export type ClientReadState = Schemas["ClientReadState"];
export type ClientRealtimeMessage = Schemas["ClientRealtimeMessage"];
export type Problem = Schemas["Problem"];

export type Fetch = (url: string, init: RequestInit) => Promise<Response>;

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
  ) {
    super(`${status} ${code}`);
  }
}

export interface Request {
  method?: string;
  json?: unknown;
  body?: Blob;
  headers?: Record<string, string>;
  token?: string | null;
  blob?: boolean;
}

export async function call<T>(
  fetcher: Fetch,
  server: string,
  path: string,
  { method = "GET", json, body, headers = {}, token, blob }: Request = {},
): Promise<T> {
  headers = { ...headers };
  if (token) headers.Authorization = `Bearer ${token}`;
  let payload: BodyInit | undefined = body;
  if (json !== undefined) {
    headers["Content-Type"] = "application/json";
    payload = JSON.stringify(json);
  }
  let response: Response;
  try {
    response = await fetcher(server + path, { method, headers, body: payload, credentials: "omit" });
  } catch {
    throw new ApiError(0, "network");
  }
  if (!response.ok) {
    let code = "error";
    try {
      code = ((await response.json()) as Problem).code ?? code;
    } catch {}
    throw new ApiError(response.status, code);
  }
  if (response.status === 204) return undefined as T;
  return (blob ? await response.blob() : await response.json()) as T;
}
