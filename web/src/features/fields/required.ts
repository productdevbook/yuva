import { ApiError } from "@/lib/api"

type Ask = (conversationId: string, field?: string) => Promise<boolean>

let asker: Ask | null = null

export function setRequiredAsker(ask: Ask | null) {
  asker = ask
}

export function isFieldRequired(err: unknown): err is ApiError {
  return err instanceof ApiError && err.status === 409 && err.code === "field_required"
}

export async function withRequiredFields<T>(conversationId: string, run: () => Promise<T>): Promise<T> {
  for (let attempt = 0; ; attempt++) {
    try {
      return await run()
    } catch (err) {
      if (!isFieldRequired(err) || !asker || attempt >= 5) throw err
      if (!(await asker(conversationId, err.field))) throw err
    }
  }
}
