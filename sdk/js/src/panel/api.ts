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
export type ClientMessage = Schemas["ClientMessage"];
export type ClientMessagePage = Schemas["ClientMessagePage"];
export type ClientAttachment = Schemas["ClientAttachment"];
export type ClientReadState = Schemas["ClientReadState"];
export type ClientRealtimeMessage = Schemas["ClientRealtimeMessage"];
export type Problem = Schemas["Problem"];

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
  form?: FormData;
  token?: string | null;
  blob?: boolean;
}

export async function call<T>(server: string, path: string, { method = "GET", json, form, token, blob }: Request = {}): Promise<T> {
  const headers: Record<string, string> = {};
  if (token) headers.Authorization = `Bearer ${token}`;
  let body: BodyInit | undefined;
  if (form) body = form;
  else if (json !== undefined) {
    headers["Content-Type"] = "application/json";
    body = JSON.stringify(json);
  }
  let response: Response;
  try {
    response = await fetch(server + path, { method, headers, body, credentials: "omit" });
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
