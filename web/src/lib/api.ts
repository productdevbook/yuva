import { useQuery } from "@tanstack/react-query"

export class ApiError extends Error {
  status: number

  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

export async function request<T>(path: string): Promise<T> {
  const res = await fetch(path, { headers: { Accept: "application/json" }, credentials: "same-origin" })
  if (!res.ok) throw new ApiError(res.status, `HTTP ${res.status}`)
  return (await res.json()) as T
}

export function useVersion() {
  return useQuery({
    queryKey: ["/v1/version"],
    queryFn: () => request<{ version: string }>("/v1/version"),
    staleTime: Infinity,
    retry: false,
  })
}
