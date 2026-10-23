import type { IssueProvider } from "@/lib/api"

export const PROVIDERS: IssueProvider[] = ["github", "linear"]

export function providerName(p: IssueProvider) {
  return p === "github" ? "GitHub" : "Linear"
}
