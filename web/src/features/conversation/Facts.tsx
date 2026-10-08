import { useLingui } from "@lingui/react/macro"

import type { Contact, Conversation } from "@/lib/api"

function shown(v: unknown) {
  if (typeof v === "string") return v.trim() || null
  if (typeof v === "number") return String(v)
  if (typeof v === "boolean") return null
  return null
}

function labelOf(key: string) {
  const s = key.replace(/[_-]+/g, " ").trim()
  return s.charAt(0).toLocaleUpperCase() + s.slice(1)
}

export type Fact = [label: string, value: string, raw?: boolean]

export function useFacts(c: Conversation, contact: Contact | undefined, others: number | undefined): Fact[] {
  const { t } = useLingui()
  const facts: Fact[] = []
  const f = c.feedback
  if (f) {
    const app = [f.app_version, f.build && `(${f.build})`].filter(Boolean).join(" ")
    const device = [f.device_model, [f.os, f.os_version].filter(Boolean).join(" ")].filter(Boolean).join(" · ")
    if (device) facts.push([t`Device`, device])
    if (app) facts.push([t`Version`, app])
    if (f.screen) facts.push([t`Screen`, f.screen])
  }
  if (contact?.emails[0]) facts.push([t`E-mail`, contact.emails[0]])
  for (const [k, v] of Object.entries(contact?.attributes ?? {})) {
    const value = shown(v)
    if (value) facts.push([labelOf(k), value, true])
  }
  const limit = others === undefined ? 4 : 3
  const out = facts.slice(0, limit)
  if (others !== undefined) {
    out.push([t`History`, others === 0 ? t`First conversation` : others === 1 ? t`1 other conversation` : t`${others} other conversations`])
  }
  return out
}

export function Facts({ facts, onOpen }: { facts: Fact[]; onOpen: () => void }) {
  const { t } = useLingui()
  if (facts.length === 0) return null
  return (
    <button
      type="button"
      onClick={onOpen}
      className="mt-5 block w-full rounded-[14px] border bg-card text-start transition-colors hover:border-input"
      title={t`Contact details`}
      data-testid="facts"
    >
      <dl className="grid grid-cols-[repeat(auto-fit,minmax(130px,1fr))] phone:grid-cols-2 [&>div+div]:border-s phone:[&>div:nth-child(3)]:border-s-0 phone:[&>div:nth-child(n+3)]:border-t">
        {facts.map(([k, v, raw]) => (
          <div key={k} className="min-w-0 px-3.5 py-2.5">
            <dt className={raw ? "truncate text-[11px] text-faint" : "truncate text-[11px] tracking-[0.04em] text-faint uppercase"}>{k}</dt>
            <dd className="mt-0.5 truncate text-[13px]" title={v}>
              {v}
            </dd>
          </div>
        ))}
      </dl>
    </button>
  )
}
