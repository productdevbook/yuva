import fs from "node:fs"
import path from "node:path"
import { parse } from "yaml"

type Obj = Record<string, any>

export type Operation = { id: string; method: string; path: string; summary: string; description: string }
export type Tag = { name: string; operations: Operation[] }

const methods = ["get", "post", "put", "patch", "delete"]
export const specPath = path.resolve("../openapi/openapi.yaml")

let cached: Obj | undefined
export function spec(): Obj {
  cached ??= parse(fs.readFileSync(specPath, "utf8"))
  return cached!
}

export function tags(): Tag[] {
  const s = spec()
  const byTag = new Map<string, Operation[]>()
  for (const [p, item] of Object.entries((s.paths ?? {}) as Obj)) {
    for (const method of methods) {
      const op = item[method]
      if (!op) continue
      const tag = op.tags?.[0] ?? "other"
      const entry = { id: op.operationId ?? `${method}-${p}`, method: method.toUpperCase(), path: p, summary: op.summary ?? "", description: op.description ?? "" }
      byTag.set(tag, [...(byTag.get(tag) ?? []), entry])
    }
  }
  return [...byTag.entries()].map(([name, operations]) => ({ name, operations }))
}

export const tagTitle = (name: string) => name.replace(/-/g, " ").replace(/\bapi\b/g, "API").replace(/^./, (c) => c.toUpperCase())
