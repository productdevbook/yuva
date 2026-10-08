import fs from "node:fs"
import path from "node:path"
import { marked } from "marked"
import { parse } from "yaml"

type Obj = Record<string, any>

export type Param = { name: string; in: string; required: boolean; type: string; description: string }
export type Field = { name: string; type: string; required: boolean; description: string }
export type Body = { contentType: string; schema: string; fields: Field[] }
export type Response = { status: string; description: string; schema?: string }
export type Operation = {
  id: string
  method: string
  path: string
  summary: string
  description: string
  auth: string[]
  params: Param[]
  body?: Body
  responses: Response[]
}
export type Tag = { name: string; description: string; operations: Operation[] }
export type Event = { name: string; summary: string; description: string; schema?: string; fields: Field[] }

const methods = ["get", "post", "put", "patch", "delete"]
const html = (md: string | undefined) => (md ? marked.parse(md, { async: false }) : "")
const inline = (md: string | undefined) => (md ? marked.parseInline(md, { async: false }) : "")

let cached: Obj | undefined
export function spec(): Obj {
  cached ??= parse(fs.readFileSync(path.resolve("../openapi/openapi.yaml"), "utf8"))
  return cached!
}

function resolve(node: Obj | undefined): Obj | undefined {
  let current = node
  while (current?.$ref) {
    const parts = (current.$ref as string).replace(/^#\//, "").split("/")
    current = parts.reduce((o: Obj | undefined, k) => o?.[k], spec())
  }
  return current
}

const refName = (node: Obj | undefined) => (node?.$ref ? (node.$ref as string).split("/").at(-1) : undefined)

export function typeOf(schema: Obj | undefined): string {
  if (!schema) return ""
  const name = refName(schema)
  if (name) return name
  if (schema.allOf) return schema.allOf.map(typeOf).filter(Boolean).join(" & ")
  if (schema.oneOf || schema.anyOf) return (schema.oneOf ?? schema.anyOf).map(typeOf).join(" | ")
  if (schema.enum) return schema.enum.map((v: unknown) => JSON.stringify(v)).join(" | ")
  const types = ([] as string[]).concat(schema.type ?? [])
  if (types.includes("array")) return `${typeOf(schema.items)}[]`
  const base = types.join(" | ") || "object"
  return schema.format ? `${base} (${schema.format})` : base
}

function fields(schema: Obj | undefined, seen = new Set<string>()): Field[] {
  const s = resolve(schema)
  if (!s) return []
  if (s.allOf) return s.allOf.flatMap((part: Obj) => fields(part, seen))
  const required = new Set<string>(s.required ?? [])
  return Object.entries((s.properties ?? {}) as Obj).map(([name, prop]) => ({
    name,
    type: typeOf(prop),
    required: required.has(name),
    description: inline(resolve(prop)?.description ?? prop.description),
  }))
}

function auth(op: Obj): string[] {
  const security: Obj[] = op.security ?? spec().security ?? []
  if (security.length === 0) return ["none"]
  return security.flatMap((s) => Object.keys(s)).filter((v, i, a) => a.indexOf(v) === i)
}

function operation(p: string, method: string, op: Obj, shared: Obj[]): Operation {
  const params = [...shared, ...(op.parameters ?? [])].map(resolve).filter(Boolean) as Obj[]
  const bodyNode = resolve(op.requestBody)
  const [contentType, media] = Object.entries((bodyNode?.content ?? {}) as Obj)[0] ?? []
  return {
    id: op.operationId ?? `${method}-${p}`,
    method: method.toUpperCase(),
    path: p,
    summary: op.summary ?? "",
    description: html(op.description),
    auth: auth(op),
    params: params.map((x) => ({ name: x.name, in: x.in, required: !!x.required, type: typeOf(x.schema), description: inline(x.description) })),
    body: contentType ? { contentType, schema: typeOf(media?.schema), fields: fields(media?.schema) } : undefined,
    responses: Object.entries((op.responses ?? {}) as Obj).map(([status, r]) => {
      const resolved = resolve(r)
      const schema = Object.values((resolved?.content ?? {}) as Obj)[0]?.schema
      return { status, description: inline(resolved?.description), schema: schema ? typeOf(schema) : undefined }
    }),
  }
}

export function tags(): Tag[] {
  const s = spec()
  const byTag = new Map<string, Operation[]>()
  for (const [p, item] of Object.entries((s.paths ?? {}) as Obj)) {
    for (const method of methods) {
      const op = item[method]
      if (!op) continue
      const tag = op.tags?.[0] ?? "other"
      byTag.set(tag, [...(byTag.get(tag) ?? []), operation(p, method, op, item.parameters ?? [])])
    }
  }
  const declared = ((s.tags ?? []) as Obj[]).map((t) => t.name as string)
  const names = [...declared, ...[...byTag.keys()].filter((n) => !declared.includes(n))]
  return names
    .filter((name) => byTag.has(name))
    .map((name) => ({
      name,
      description: html(((s.tags ?? []) as Obj[]).find((t) => t.name === name)?.description),
      operations: byTag.get(name)!,
    }))
}

export function events(): Event[] {
  return Object.entries((spec().webhooks ?? {}) as Obj).map(([name, item]) => {
    const op = item.post ?? Object.values(item)[0]
    const schema = Object.values((resolve(op?.requestBody)?.content ?? {}) as Obj)[0]?.schema
    return { name, summary: op?.summary ?? "", description: html(op?.description), schema: schema ? typeOf(schema) : undefined, fields: fields(schema) }
  })
}

export function info() {
  const s = spec()
  return { title: s.info?.title as string, version: s.info?.version as string, description: html(s.info?.description), schemes: s.components?.securitySchemes as Obj }
}

export const tagTitle = (name: string) => name.replace(/-/g, " ").replace(/\bapi\b/g, "API").replace(/^./, (c) => c.toUpperCase())
