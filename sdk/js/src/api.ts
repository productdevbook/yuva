import createClient, { type Client, type ClientOptions } from "openapi-fetch";
import type { components, operations, paths } from "./schema.gen";

export type { components, operations, paths };
export type YuvaApi = Client<paths>;

export interface YuvaApiOptions extends Omit<ClientOptions, "baseUrl"> {
  server: string;
  apiKey: string;
}

export function createYuvaApi({ server, apiKey, headers, ...options }: YuvaApiOptions): YuvaApi {
  return createClient<paths>({
    ...options,
    baseUrl: server.replace(/\/+$/, ""),
    headers: { ...(headers as Record<string, string> | undefined), Authorization: `Bearer ${apiKey}` },
  });
}
