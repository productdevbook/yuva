import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { LoaderIcon, PlusIcon, TriangleAlertIcon, UsersIcon, XIcon } from "lucide-react"
import { useState } from "react"
import { Link } from "react-router"

import { CheckItem, CheckList, Notice } from "@/components/common"
import { useLanguageName } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { useNumber } from "@/features/broadcasts/BroadcastParts"
import { listLink, useLists } from "@/features/broadcasts/queries"
import { Card } from "@/features/settings/ui"
import type {
  BroadcastAudience,
  BroadcastAudienceCount,
  BroadcastAudienceExcluded,
  BroadcastCondition,
  BroadcastConditionOp,
} from "@/lib/api"

const OPS: BroadcastConditionOp[] = ["eq", "neq", "contains", "gt", "lt", "in", "exists", "not_exists"]
const ATTRIBUTE = /^[A-Za-z0-9_.-]{1,64}$/
const LOCALE = /^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})*$/
const MAX_LISTS = 10
const MAX_CONDITIONS = 10

export type Audience = BroadcastAudience

function useOpNames(): Record<BroadcastConditionOp, string> {
  const { t } = useLingui()
  return {
    eq: t`is`,
    neq: t`is not`,
    contains: t`contains`,
    gt: t`is more than`,
    lt: t`is less than`,
    in: t`is one of`,
    exists: t`is set`,
    not_exists: t`is not set`,
  }
}

export function useExcludedNames(): Record<keyof BroadcastAudienceExcluded, string> {
  const { t } = useLingui()
  return {
    unsubscribed: t`Unsubscribed`,
    pending: t`Not confirmed yet`,
    suppressed: t`Bounced or reported as spam`,
    opted_out: t`Objected to all mail`,
    blocked: t`Blocked contacts`,
    filtered: t`Left out by a condition or language`,
    duplicate: t`On several lists, one mail each`,
  }
}

export function conditionValid(c: BroadcastCondition) {
  if (!ATTRIBUTE.test(c.attribute)) return false
  if (c.op === "exists" || c.op === "not_exists") return true
  if (c.op === "in") return (c.values ?? []).length > 0
  return (c.value ?? "") !== ""
}

function ConditionRow({
  c,
  index,
  disabled,
  onChange,
  onRemove,
}: {
  c: BroadcastCondition
  index: number
  disabled: boolean
  onChange: (c: BroadcastCondition) => void
  onRemove: () => void
}) {
  const { t } = useLingui()
  const ops = useOpNames()
  const [raw, setRaw] = useState((c.values ?? []).join(", "))
  const bad = c.attribute !== "" && !ATTRIBUTE.test(c.attribute)
  const n = index + 1
  return (
    <li className="flex flex-wrap items-start gap-2 px-3.5 py-2.5" data-testid="audience-condition">
      <Input
        value={c.attribute}
        disabled={disabled}
        maxLength={64}
        placeholder={t`attribute, e.g. plan`}
        aria-label={t`Attribute of condition ${n}`}
        aria-invalid={bad || undefined}
        className="min-w-32 flex-1 font-mono text-small"
        onChange={(e) => onChange({ ...c, attribute: e.target.value.trim() })}
        data-testid="condition-attribute"
      />
      <Select
        value={c.op}
        items={ops}
        disabled={disabled}
        onValueChange={(v) => {
          const op = v as BroadcastConditionOp
          onChange(
            op === "in"
              ? { attribute: c.attribute, op, values: raw ? splitValues(raw) : c.value ? [c.value] : [] }
              : op === "exists" || op === "not_exists"
                ? { attribute: c.attribute, op }
                : { attribute: c.attribute, op, value: c.value ?? c.values?.[0] ?? "" }
          )
        }}
      >
        <SelectTrigger className="w-40 min-w-0" aria-label={t`Comparison of condition ${n}`}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {OPS.map((op) => (
            <SelectItem key={op} value={op}>
              {ops[op]}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {c.op === "in" ? (
        <Input
          value={raw}
          disabled={disabled}
          placeholder={t`pro, team`}
          aria-label={t`Values of condition ${n}, separated by commas`}
          className="min-w-32 flex-1"
          onChange={(e) => {
            setRaw(e.target.value)
            onChange({ ...c, values: splitValues(e.target.value) })
          }}
          data-testid="condition-values"
        />
      ) : c.op !== "exists" && c.op !== "not_exists" ? (
        <Input
          value={c.value ?? ""}
          disabled={disabled}
          maxLength={500}
          placeholder={t`value`}
          aria-label={t`Value of condition ${n}`}
          className="min-w-32 flex-1"
          onChange={(e) => onChange({ ...c, value: e.target.value })}
          data-testid="condition-value"
        />
      ) : null}
      {!disabled && (
        <Button
          type="button"
          variant="ghost"
          size="icon"
          onClick={onRemove}
          aria-label={t`Remove condition ${n}`}
          data-testid="condition-remove"
        >
          <XIcon />
        </Button>
      )}
    </li>
  )
}

function splitValues(raw: string) {
  return raw
    .split(",")
    .map((v) => v.trim())
    .filter(Boolean)
    .slice(0, 50)
}

function Locales({
  value,
  disabled,
  onChange,
}: {
  value: string[]
  disabled: boolean
  onChange: (v: string[]) => void
}) {
  const { t } = useLingui()
  const [draft, setDraft] = useState("")
  const tag = draft.trim()
  const valid = LOCALE.test(tag) && !value.includes(tag) && value.length < 20
  const add = () => {
    if (!valid) return
    onChange([...value, tag])
    setDraft("")
  }
  return (
    <div className="flex flex-col gap-2" data-testid="audience-locales">
      <span className="text-body font-medium">
        <Trans>Languages</Trans>
      </span>
      <div className="flex flex-wrap items-center gap-1.5">
        {value.map((lang) => (
          <span
            key={lang}
            className="inline-flex items-center gap-1 rounded-full border bg-card py-0.5 ps-2.5 pe-1 text-small"
            data-testid="audience-locale"
          >
            <span dir="ltr">{lang}</span>
            {!disabled && (
              <button
                type="button"
                className="grid size-5 place-items-center rounded-full text-faint hover:bg-muted hover:text-foreground"
                onClick={() => onChange(value.filter((x) => x !== lang))}
                aria-label={t`Remove ${lang}`}
              >
                <XIcon className="size-3" />
              </button>
            )}
          </span>
        ))}
        {!disabled && (
          <form
            className="flex items-center gap-1.5"
            onSubmit={(e) => {
              e.preventDefault()
              add()
            }}
          >
            <Input
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              placeholder={t`tr, en-GB`}
              aria-label={t`Language to keep`}
              className="h-8 w-28"
              maxLength={35}
              data-testid="audience-locale-input"
            />
            <Button type="submit" variant="outline" size="sm" disabled={!valid}>
              <Trans>Add</Trans>
            </Button>
          </form>
        )}
      </div>
      <p className="text-small text-faint">
        {value.length === 0 ? (
          <Trans>Every language. Add a language to keep only contacts who use it; tr also keeps tr-TR.</Trans>
        ) : (
          <Trans>Only contacts whose language is one of these.</Trans>
        )}
      </p>
    </div>
  )
}

export function AudienceBuilder({
  inboxId,
  value,
  onChange,
  disabled,
  listsLocked = disabled,
  hideLists = false,
}: {
  inboxId: string
  value: Audience
  onChange: (a: Audience) => void
  disabled: boolean
  listsLocked?: boolean
  hideLists?: boolean
}) {
  const { t } = useLingui()
  const num = useNumber()
  const lists = useLists(inboxId, false)
  const items = lists.data?.pages.flatMap((p) => p.items) ?? []
  const chosen = new Set(value.list_ids)
  const full = value.list_ids.length >= MAX_LISTS
  return (
    <div className="flex flex-col gap-5">
      {hideLists ? null : lists.isPending ? (
        <Skeleton className="h-24 rounded-xl" />
      ) : items.length === 0 ? (
        <Notice tone="warning" icon={UsersIcon} data-testid="audience-no-lists">
          <p>
            <Trans>This inbox has no mailing list yet. A broadcast goes only to people who subscribed to a list.</Trans>
          </p>
          <Link to="/broadcasts/lists" className="w-fit text-brand underline-offset-4 hover:underline">
            <Trans>Go to the lists</Trans>
          </Link>
        </Notice>
      ) : (
        <CheckList
          labelId="audience-lists"
          label={<Trans>Lists</Trans>}
          hint={
            <p className="text-small text-faint">
              <Trans>Someone on several lists gets one mail, under the first list they are on in this order.</Trans>
            </p>
          }
        >
          {items.map((l) => {
            const subscribed = num(l.counts.subscribed)
            const position = value.list_ids.indexOf(l.id) + 1
            return (
              <CheckItem
                key={l.id}
                checked={chosen.has(l.id)}
                disabled={listsLocked || (!chosen.has(l.id) && full)}
                onChange={(on) =>
                  onChange({
                    ...value,
                    list_ids: on ? [...value.list_ids, l.id] : value.list_ids.filter((id) => id !== l.id),
                  })
                }
                testId="audience-list"
              >
                <span className="flex min-w-0 items-center gap-2">
                  <span className="min-w-0 flex-1 truncate">{l.name}</span>
                  {position > 0 && value.list_ids.length > 1 && (
                    <span className="shrink-0 rounded-full bg-muted px-1.5 text-caption text-muted-foreground tabular-nums">
                      {position}
                    </span>
                  )}
                </span>
                <span className="text-small text-faint">{t`${subscribed} subscribed`}</span>
              </CheckItem>
            )
          })}
        </CheckList>
      )}
      <div className="flex flex-col gap-2" role="group" aria-labelledby="audience-conditions">
        <span id="audience-conditions" className="text-body font-medium">
          <Trans>Conditions</Trans>
        </span>
        {value.conditions.length > 0 && (
          <ul className="divide-y rounded-xl border">
            {value.conditions.map((c, i) => (
              <ConditionRow
                key={i}
                c={c}
                index={i}
                disabled={disabled}
                onChange={(next) =>
                  onChange({ ...value, conditions: value.conditions.map((x, j) => (j === i ? next : x)) })
                }
                onRemove={() => onChange({ ...value, conditions: value.conditions.filter((_, j) => j !== i) })}
              />
            ))}
          </ul>
        )}
        {!disabled && value.conditions.length < MAX_CONDITIONS && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="w-fit"
            onClick={() =>
              onChange({ ...value, conditions: [...value.conditions, { attribute: "", op: "eq", value: "" }] })
            }
            data-testid="audience-add-condition"
          >
            <PlusIcon />
            <Trans>Add a condition</Trans>
          </Button>
        )}
        <p className="text-small text-faint">
          {value.conditions.length === 0 ? (
            hideLists ? (
              <Trans>Everyone who signs in. Conditions keep only contacts whose attributes match, all of them.</Trans>
            ) : (
              <Trans>Everyone on the lists. Conditions keep only contacts whose attributes match, all of them.</Trans>
            )
          ) : (
            <Trans>
              A contact must match every condition. Without the attribute, only “is not” and “is not set” match.
            </Trans>
          )}
        </p>
      </div>
      <Locales value={value.locales} disabled={disabled} onChange={(locales) => onChange({ ...value, locales })} />
    </div>
  )
}

export function AudienceCount({
  count,
  loading,
  stale,
  error,
  listNames,
}: {
  count: BroadcastAudienceCount | undefined
  loading: boolean
  stale: boolean
  error: React.ReactNode
  listNames: Map<string, string>
}) {
  const { t } = useLingui()
  const num = useNumber()
  const names = useExcludedNames()
  const languageName = useLanguageName()
  if (!count) {
    return (
      <Card className="bg-surface" data-testid="audience-count">
        {loading ? (
          <Skeleton className="h-16 w-full" />
        ) : (
          error || (
            <p className="text-body text-faint">
              <Trans>Pick at least one list to see who gets the mail.</Trans>
            </p>
          )
        )}
      </Card>
    )
  }
  const recipients = num(count.recipients)
  const always: (keyof BroadcastAudienceExcluded)[] = ["unsubscribed", "pending", "suppressed", "filtered"]
  const excluded = (Object.keys(names) as (keyof BroadcastAudienceExcluded)[]).filter(
    (k) => always.includes(k) || count.excluded[k] > 0
  )
  const people = (Object.keys(names) as (keyof BroadcastAudienceExcluded)[])
    .filter((k) => k !== "duplicate")
    .reduce((n, k) => n + count.excluded[k], 0)
  const left = num(people)
  const total = num(count.recipients + people)
  const missing = Object.entries(count.missing_merge_fields).filter(([, n]) => n > 0)
  return (
    <Card className="bg-surface" data-testid="audience-count">
      <div className="flex items-baseline gap-2">
        <b className="text-page tabular-nums" data-testid="audience-recipients">
          {recipients}
        </b>
        <span className="text-body text-muted-foreground">
          <Plural value={count.recipients} one="recipient" other="recipients" />
        </span>
        {stale && <LoaderIcon className="ms-auto size-4 animate-spin text-faint" aria-label={t`Updating`} />}
      </div>
      {count.by_list.length > 1 && (
        <ul className="flex flex-col gap-1 text-small" data-testid="audience-by-list">
          {count.by_list.map((l) => (
            <li key={l.list_id} className="flex justify-between gap-3">
              <Link to={listLink(l.list_id)} className="min-w-0 truncate text-muted-foreground hover:text-foreground">
                {listNames.get(l.list_id) ?? "…"}
              </Link>
              <span className="tabular-nums">{num(l.recipients)}</span>
            </li>
          ))}
        </ul>
      )}
      {(count.by_locale?.length ?? 0) > 0 && (
        <div className="flex flex-col gap-1.5">
          <span className="text-small font-medium text-faint">
            <Trans>Languages</Trans>
          </span>
          <ul className="flex flex-col gap-1 text-small" data-testid="audience-by-locale">
            {count.by_locale!.map((l) => {
              const language = languageName(l.locale)
              return (
                <li key={l.locale} className="flex flex-col" data-locale={l.locale}>
                  <span className="flex justify-between gap-3">
                    <span className="min-w-0 truncate text-muted-foreground">{language}</span>
                    <span className="tabular-nums">{num(l.recipients)}</span>
                  </span>
                  {l.fallback > 0 && (
                    <span className="text-caption text-faint">
                      <Plural
                        value={l.fallback}
                        one="# of them gets it because their language has no version"
                        other="# of them get it because their language has no version"
                      />
                    </span>
                  )}
                </li>
              )
            })}
          </ul>
        </div>
      )}
      <div className="flex flex-col gap-1.5">
        <span className="text-small font-medium text-faint" data-testid="audience-left-out">
          <Trans>
            Left out: {left} of {total} people on the lists
          </Trans>
        </span>
        <ul className="flex flex-col gap-1 text-small" data-testid="audience-excluded">
          {excluded.map((k) => (
            <li key={k} className="flex justify-between gap-3" data-reason={k}>
              <span className="min-w-0 text-muted-foreground">{names[k]}</span>
              <span className="tabular-nums">{num(count.excluded[k])}</span>
            </li>
          ))}
        </ul>
      </div>
      {missing.length > 0 && (
        <Notice tone="warning" icon={TriangleAlertIcon} className="px-3 py-2.5" data-testid="audience-missing-fields">
          <p>
            <Trans>These merge fields have no fallback and would be empty for some recipients:</Trans>
          </p>
          <ul className="flex flex-col gap-0.5">
            {missing.map(([field, n]) => (
              <li key={field} className="flex justify-between gap-3 text-small">
                <code className="min-w-0 truncate font-mono" dir="ltr">
                  {field}
                </code>
                <span className="shrink-0 tabular-nums">
                  <Plural value={n} one="# person" other="# people" />
                </span>
              </li>
            ))}
          </ul>
        </Notice>
      )}
    </Card>
  )
}
