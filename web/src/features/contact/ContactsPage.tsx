import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowLeftRightIcon, BookUserIcon, ChevronDownIcon, PlusIcon, SearchIcon, XIcon } from "lucide-react"
import { useEffect, useMemo, useRef, useState } from "react"
import { Link, useNavigate, useSearchParams } from "react-router"

import { ContactAvatar, Dot, ErrorLine, toast } from "@/components/common"
import { useErrorText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import { RowCheck, SelectToggle } from "@/components/common/Selection"
import { ContactBulkBar } from "@/features/contact/ContactBulk"
import { ContactDetail } from "@/features/contact/ContactDetail"
import { MergeContactDialog } from "@/features/contact/MergeContactDialog"
import { useContactDirectory, useCreateContact } from "@/features/contact/queries"
import { contactName } from "@/features/contact/Section"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { Ago } from "@/features/inbox/PageFrame"
import { PageTabs } from "@/features/settings/ui"
import { useDebounced } from "@/hooks/use-debounced"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useSelection } from "@/hooks/use-selection"
import type { Contact } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

type Tab = "all" | "recent" | "duplicates"

const GRID =
  "grid grid-cols-[minmax(200px,2.2fr)_minmax(110px,1.2fr)_minmax(0,100px)_72px_96px] items-center gap-3 px-3 max-xl:grid-cols-[minmax(180px,2fr)_minmax(100px,1fr)_minmax(0,90px)_72px]"

const chipBase =
  "inline-flex h-[34px] shrink-0 items-center gap-1.5 rounded-[9px] border px-2.5 text-small whitespace-nowrap outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring"
const chipOff =
  "border-dashed border-input bg-transparent text-muted-foreground hover:border-faint hover:text-foreground"
const chipOn = "border-input bg-card font-medium text-foreground data-popup-open:bg-muted"
const toggleOff = "border-input bg-card text-muted-foreground hover:border-faint hover:text-foreground"
const toggleOn = "border-foreground bg-foreground text-background"
const actionInk = "h-8 rounded-[9px] bg-foreground px-3.5 font-semibold text-background hover:bg-foreground/90"
const actionOutline = "h-8 rounded-[9px] border-input bg-card px-3 font-normal"

export function countryOf(c: Contact, locale: string) {
  const attr = c.attributes?.country
  if (typeof attr === "string" && attr) return attr
  const region = c.locale?.split(/[-_]/)[1]
  if (!region) return undefined
  try {
    return new Intl.DisplayNames([locale], { type: "region" }).of(region.toUpperCase())
  } catch {
    return undefined
  }
}

const DISMISSED = "contacts-not-same"

function readDismissed(): string[] {
  try {
    const v = JSON.parse(localStorage.getItem(DISMISSED) ?? "[]")
    return Array.isArray(v) ? v.filter((x) => typeof x === "string") : []
  } catch {
    return []
  }
}

type Pair = { key: string; a: Contact; b: Contact; inbox?: string }

function useDuplicates(items: Contact[], dismissed: string[]) {
  const inboxes = useInboxes().data
  return useMemo(() => {
    const groups = new Map<string, Contact[]>()
    for (const c of items) {
      const n = c.name?.trim().toLocaleLowerCase()
      if (!n) continue
      groups.set(n, [...(groups.get(n) ?? []), c])
    }
    const pairs: Pair[] = []
    for (const g of groups.values()) {
      if (g.length < 2) continue
      const sorted = [...g].sort((x, y) => (y.activity?.conversations ?? 0) - (x.activity?.conversations ?? 0))
      const [a, ...rest] = sorted
      for (const b of rest) {
        const key = [a.id, b.id].sort().join(":")
        if (dismissed.includes(key)) continue
        const shared = a.inboxes.find((i) => b.inboxes.some((j) => j.id === i.id))
        pairs.push({ key, a, b, inbox: shared ? inboxes?.find((i) => i.id === shared.id)?.name : undefined })
      }
    }
    return pairs
  }, [items, dismissed, inboxes])
}

function FilterChip({
  value,
  onChange,
  options,
  placeholder,
  anyLabel,
  ariaLabel,
  testId,
}: {
  value: string
  onChange: (v: string) => void
  options: { value: string; label: string }[]
  placeholder: React.ReactNode
  anyLabel: React.ReactNode
  ariaLabel: string
  testId: string
}) {
  const current = options.find((o) => o.value === value)
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={<button type="button" className={cn(chipBase, current ? chipOn : chipOff)} />}
        aria-label={ariaLabel}
        data-testid={testId}
      >
        {current ? (
          <>
            <span className="max-w-28 truncate">{current.label}</span>
            <ChevronDownIcon className="size-3 text-faint" />
          </>
        ) : (
          <>
            <PlusIcon className="size-3.5" />
            {placeholder}
          </>
        )}
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="min-w-44">
        <DropdownMenuRadioGroup value={value} onValueChange={(v) => onChange(String(v))}>
          <DropdownMenuRadioItem value="">{anyLabel}</DropdownMenuRadioItem>
          {options.map((o) => (
            <DropdownMenuRadioItem key={o.value} value={o.value}>
              <span className="truncate">{o.label}</span>
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function ToggleChip({
  on,
  onClick,
  testId,
  children,
}: {
  on: boolean
  onClick: () => void
  testId: string
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      aria-pressed={on}
      onClick={onClick}
      className={cn(chipBase, on ? toggleOn : toggleOff)}
      data-testid={testId}
    >
      {children}
    </button>
  )
}

function Tag({ tone, children }: { tone: "danger" | "warn"; children: React.ReactNode }) {
  return (
    <span
      className={cn(
        "inline-flex h-5 shrink-0 items-center rounded-md px-[7px] text-[11px] font-semibold",
        tone === "danger" ? "bg-destructive/10 text-destructive" : "bg-warning/10 text-warning"
      )}
    >
      {children}
    </span>
  )
}

function Row({
  c,
  current,
  cursor,
  search,
  check,
}: {
  c: Contact
  current: boolean
  cursor: boolean
  search: string
  check?: { checked: boolean; onToggle: (range: boolean) => void }
}) {
  const { t, i18n } = useLingui()
  const inboxes = useInboxes().data
  const name = contactName(c, t`Unnamed contact`)
  const sub = c.emails[0] && c.emails[0] !== name ? c.emails[0] : (c.external_ids[0]?.external_id ?? "")
  const seen = c.activity?.last_seen_at
  return (
    <li className="flex items-center">
      {check && <RowCheck className="ms-4" checked={check.checked} name={name} onToggle={check.onToggle} />}
      <Link
        to={`/contacts/${c.id}${search}`}
        onClick={
          check
            ? (e) => {
                e.preventDefault()
                check.onToggle(e.shiftKey)
              }
            : undefined
        }
        className={cn(
          "min-w-0 flex-1",
          GRID,
          "relative min-h-14 border-b border-b-muted py-2 outline-none transition-colors hover:bg-muted/60 focus-visible:bg-muted/60",
          current && "rounded-[10px] border-b-transparent bg-card shadow-[0_0_0_1px_var(--border)] hover:bg-card",
          cursor && !current && "bg-muted/60",
          (current || cursor) &&
            "before:absolute before:inset-y-3 before:start-1 before:w-[3px] before:rounded-[3px] before:bg-brand"
        )}
        aria-current={current ? "page" : undefined}
        data-testid="contact-row"
      >
        <span className="flex min-w-0 items-center gap-3">
          <ContactAvatar id={c.id} name={name} className="size-[34px] shrink-0 text-caption font-semibold" />
          <span className="flex min-w-0 flex-col">
            <span className="flex min-w-0 items-center gap-1.5">
              <span className="truncate text-[13.5px] font-semibold">{name}</span>
              {c.blocked && (
                <Tag tone="danger">
                  <Trans>Blocked</Trans>
                </Tag>
              )}
              {c.undeliverable.length > 0 && (
                <Tag tone="warn">
                  <Trans>Undeliverable</Trans>
                </Tag>
              )}
            </span>
            <span className="truncate text-caption text-faint">{sub || <Trans>Visitor</Trans>}</span>
          </span>
        </span>
        <span className="flex min-w-0 flex-wrap gap-x-2.5 gap-y-1 overflow-hidden">
          {c.inboxes.slice(0, 3).map((i) => (
            <span key={i.id} className="inline-flex min-w-0 items-center gap-[5px] text-caption text-muted-foreground">
              <Dot color={inboxes?.find((x) => x.id === i.id)?.branding.color} className="size-2 rounded-[3px]" />
              <span className="truncate">{i.name}</span>
            </span>
          ))}
        </span>
        <span className="truncate text-[12.5px] text-muted-foreground">
          {countryOf(c, i18n.locale) ?? <span className="text-faint">—</span>}
        </span>
        <span className="text-small text-muted-foreground tabular-nums">
          {c.activity ? new Intl.NumberFormat(i18n.locale).format(c.activity.conversations) : "—"}
        </span>
        <span className="truncate text-[12.5px] text-faint max-xl:hidden">{seen ? <Ago iso={seen} /> : "—"}</span>
      </Link>
    </li>
  )
}

function Side({ c }: { c: Contact }) {
  const { t } = useLingui()
  const name = contactName(c, t`Unnamed contact`)
  const products = c.inboxes.map((i) => i.name).join(", ")
  const n = c.activity?.conversations ?? 0
  return (
    <div className="flex min-w-0 flex-1 items-center gap-3">
      <ContactAvatar id={c.id} name={name} className="size-[34px] shrink-0 text-caption font-semibold" />
      <span className="flex min-w-0 flex-col leading-[1.35]">
        <Link to={`/contacts/${c.id}`} className="truncate text-[13.5px] font-semibold hover:underline">
          {name}
        </Link>
        <span className="truncate text-caption text-faint">{c.emails[0] ?? <Trans>no e-mail</Trans>}</span>
        <span className="truncate text-caption text-faint">
          {products ? `${products} · ` : ""}
          {n === 1 ? <Trans>1 conversation</Trans> : <Trans>{n} conversations</Trans>}
        </span>
      </span>
    </div>
  )
}

function PairCard({ pair, onDismiss }: { pair: Pair; onDismiss: () => void }) {
  const { canManage } = useSession()
  const [merging, setMerging] = useState(false)
  const shared = pair.inbox
  return (
    <li
      className="grid grid-cols-[minmax(0,1fr)_40px_minmax(0,1fr)_auto] items-center gap-3 rounded-[14px] border bg-card px-4 py-3.5 max-xl:grid-cols-[minmax(0,1fr)_40px_minmax(0,1fr)] max-xl:[&>:last-child]:col-span-full"
      data-testid="duplicate-pair"
    >
      <Side c={pair.a} />
      <ArrowLeftRightIcon className="mx-auto size-4 text-faint" aria-hidden />
      <Side c={pair.b} />
      <div className="flex flex-col items-end gap-1.5 max-xl:items-start">
        <p className="text-caption text-faint">
          {shared ? <Trans>Same name, both wrote to {shared}.</Trans> : <Trans>Same name.</Trans>}
        </p>
        <div className="flex gap-1.5">
          <Button variant="outline" className={actionOutline} onClick={onDismiss} data-testid="duplicate-dismiss">
            <Trans>Not the same</Trans>
          </Button>
          {canManage && (
            <Button className={actionInk} onClick={() => setMerging(true)} data-testid="duplicate-merge">
              <Trans>Merge…</Trans>
            </Button>
          )}
        </div>
      </div>
      {canManage && (
        <MergeContactDialog target={pair.a} initialSource={pair.b} open={merging} onOpenChange={setMerging} />
      )}
    </li>
  )
}

function AddContactDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const navigate = useNavigate()
  const errorText = useErrorText()
  const create = useCreateContact()
  const [name, setName] = useState("")
  const [email, setEmail] = useState("")
  const ready = !!name.trim() || !!email.trim()
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent data-testid="add-contact">
        <DialogHeader>
          <DialogTitle>
            <Trans>Add contact</Trans>
          </DialogTitle>
          <DialogDescription>
            <Trans>Someone who has not written yet. You can start a conversation with them next.</Trans>
          </DialogDescription>
        </DialogHeader>
        <form
          id="add-contact-form"
          className="flex flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault()
            if (!ready) return
            create.mutate(
              { name: name.trim() || undefined, emails: email.trim() ? [email.trim()] : undefined },
              {
                onSuccess: (c) => {
                  onOpenChange(false)
                  setName("")
                  setEmail("")
                  navigate(`/contacts/${c.id}`)
                  toast(<Trans>Contact added. Use New conversation to write to them.</Trans>)
                },
                onError: (err) => toast(errorText(err)),
              }
            )
          }}
        >
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="add-contact-name">
              <Trans>Name</Trans>
            </Label>
            <Input
              id="add-contact-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              maxLength={200}
              className="h-[38px] rounded-[9px]"
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="add-contact-email">
              <Trans>E-mail</Trans>
            </Label>
            <Input
              id="add-contact-email"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="h-[38px] rounded-[9px]"
            />
          </div>
        </form>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            <Trans>Cancel</Trans>
          </Button>
          <Button
            type="submit"
            form="add-contact-form"
            disabled={!ready || create.isPending}
            data-testid="add-contact-create"
          >
            <Trans>Add contact</Trans>
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export function ContactsPage({ contactId }: { contactId?: string }) {
  const { t, i18n } = useLingui()
  const { canManage } = useSession()
  const navigate = useNavigate()
  const [params, setParams] = useSearchParams()
  const tabParam = params.get("tab")
  const tab: Tab = tabParam === "recent" || tabParam === "duplicates" ? tabParam : "all"
  const [global] = useInboxFilter()
  const inboxes = useInboxes().data
  const picked = params.get("inbox") ?? ""
  const chosen = inboxes?.some((i) => i.id === picked) ? picked : ""
  const inboxId = chosen || global
  const product = inboxes?.find((i) => i.id === inboxId)?.name
  const q = params.get("q") ?? ""
  const country = params.get("country") ?? ""
  const blockedOnly = params.get("blocked") === "1"
  const undeliverableOnly = params.get("undeliverable") === "1"
  const fullPage = params.get("view") === "page" && !!contactId && tab !== "duplicates"
  const query = useDebounced(q.trim(), 250)
  const directory = useContactDirectory(query, "all", inboxId, tab === "recent" ? "last_seen" : "created")
  const everyone = useContactDirectory("", "all", inboxId, "last_seen")
  const loaded = useMemo(() => directory.data?.pages.flatMap((p) => p.items) ?? [], [directory.data])
  const all = useMemo(() => everyone.data?.pages.flatMap((p) => p.items) ?? [], [everyone.data])
  const countries = useMemo(
    () => [...new Set(loaded.map((c) => countryOf(c, i18n.locale)).filter((x): x is string => !!x))].sort(),
    [loaded, i18n.locale]
  )
  const items = useMemo(
    () =>
      loaded.filter(
        (c) =>
          (!country || countryOf(c, i18n.locale) === country) &&
          (!blockedOnly || c.blocked) &&
          (!undeliverableOnly || c.undeliverable.length > 0)
      ),
    [loaded, country, blockedOnly, undeliverableOnly, i18n.locale]
  )
  const [dismissed, setDismissed] = useState(readDismissed)
  const pairs = useDuplicates(all, dismissed)
  const [cursor, setCursor] = useState(-1)
  const [adding, setAdding] = useState(false)
  const selection = useSelection(
    items.map((c) => c.id),
    `${tab}|${query}|${inboxId}|${country}|${blockedOnly}|${undeliverableOnly}`
  )
  const list = useRef<HTMLUListElement>(null)
  const search = useRef<HTMLInputElement>(null)
  const listParams = new URLSearchParams(params)
  listParams.delete("view")
  const suffix = listParams.toString() ? `?${listParams.toString()}` : ""
  const shown = tab === "duplicates" ? undefined : (contactId ?? items[0]?.id)
  const anyFilter = !!(q || chosen || country || blockedOnly || undeliverableOnly)
  const shownCount = new Intl.NumberFormat(i18n.locale).format(items.length)

  const set = (key: string, value: string) =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev)
        if (value) next.set(key, value)
        else next.delete(key)
        return next
      },
      { replace: true }
    )
  const tabTo = (k: Tab) => {
    const next = new URLSearchParams(params)
    next.delete("view")
    if (k === "all") next.delete("tab")
    else next.set("tab", k)
    return { search: next.toString() ? `?${next.toString()}` : "" }
  }
  const clearFilters = () =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev)
        for (const k of ["q", "inbox", "country", "blocked", "undeliverable"]) next.delete(k)
        return next
      },
      { replace: true }
    )

  const dismiss = (key: string) => {
    const next = [...dismissed, key]
    setDismissed(next)
    try {
      localStorage.setItem(DISMISSED, JSON.stringify(next))
    } catch {
      return
    }
  }

  useHotkeys(
    tab === "duplicates" || fullPage
      ? {}
      : {
          j: () => setCursor((i) => Math.min(items.length - 1, i + 1)),
          k: () => setCursor((i) => Math.max(0, i - 1)),
          ArrowDown: () => setCursor((i) => Math.min(items.length - 1, i + 1)),
          ArrowUp: () => setCursor((i) => Math.max(0, i - 1)),
          Enter: () => cursor >= 0 && items[cursor] && navigate(`/contacts/${items[cursor].id}${suffix}`),
          "/": () => search.current?.focus(),
          ...(canManage
            ? {
                x: () => {
                  const id = items[cursor]?.id ?? shown
                  if (id) selection.toggle(id)
                  else selection.start()
                },
              }
            : {}),
        }
  )
  useEffect(() => {
    if (cursor >= 0)
      list.current?.querySelectorAll("[data-testid=contact-row]")[cursor]?.scrollIntoView({ block: "nearest" })
  }, [cursor])

  const header = (
    <header className="flex shrink-0 flex-col gap-3 px-6 pt-[18px]" data-testid="contacts-head">
      <div className="flex min-w-0 flex-wrap items-center gap-3">
        <div className="flex min-w-0 flex-1 basis-56 flex-col gap-0.5">
          <h1 className="text-[22px] leading-7 font-semibold tracking-[-0.01em]">
            <Trans>Contacts</Trans>
          </h1>
          <p className="truncate text-small text-faint">
            {product ? (
              <Trans>People who wrote to {product}</Trans>
            ) : (
              <Trans>Everyone who has written to your products</Trans>
            )}
          </p>
        </div>
        <Button className={actionInk} onClick={() => setAdding(true)} data-testid="add-contact-open">
          <PlusIcon className="size-3.5" aria-hidden />
          <Trans>Add contact</Trans>
        </Button>
      </div>
      <PageTabs
        label={t`Show`}
        current={tab}
        items={[
          { key: "all", to: tabTo("all"), label: <Trans>All</Trans>, testId: "contacts-filter-all" },
          {
            key: "recent",
            to: tabTo("recent"),
            label: <Trans>Recently active</Trans>,
            testId: "contacts-filter-recent",
          },
          {
            key: "duplicates",
            to: tabTo("duplicates"),
            label: <Trans>Duplicates</Trans>,
            count: everyone.data && !everyone.hasNextPage ? pairs.length : undefined,
            testId: "contacts-filter-duplicates",
          },
        ]}
      />
    </header>
  )

  const duplicates = (
    <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto px-6 pt-4 pb-6" data-testid="duplicates">
      <p className="max-w-3xl text-small text-faint">
        <Trans>
          These look like the same person. Merging keeps the left one and moves addresses, ids and conversations into
          it. Nothing is merged until you have seen what changes.
        </Trans>
      </p>
      {everyone.isPending ? (
        <Skeleton className="h-24 w-full max-w-[980px] rounded-[14px]" />
      ) : pairs.length === 0 ? (
        <p className="max-w-[980px] py-10 text-center text-small text-faint">
          <Trans>No contacts look alike right now.</Trans>
        </p>
      ) : (
        <ul className="flex max-w-[980px] flex-col gap-2.5">
          {pairs.map((p) => (
            <PairCard key={p.key} pair={p} onDismiss={() => dismiss(p.key)} />
          ))}
        </ul>
      )}
      {everyone.hasNextPage && (
        <Button
          variant="outline"
          className={cn(actionOutline, "self-start")}
          onClick={() => void everyone.fetchNextPage()}
          disabled={everyone.isFetchingNextPage}
        >
          <Trans>Look through more contacts</Trans>
        </Button>
      )}
    </div>
  )

  const toolbar = (
    <div className="flex shrink-0 flex-wrap items-center gap-2 px-6 py-3" data-testid="contacts-filters">
      <label className="flex h-[34px] max-w-[360px] min-w-[220px] flex-1 items-center gap-2 rounded-[9px] border border-input bg-card px-2.5 focus-within:border-brand focus-within:ring-[3px] focus-within:ring-brand/20">
        <SearchIcon className="pointer-events-none size-3.5 shrink-0 text-faint" aria-hidden />
        <input
          ref={search}
          value={q}
          onChange={(e) => {
            set("q", e.target.value)
            setCursor(-1)
          }}
          onKeyDown={(e) => {
            if (e.key === "Escape") {
              if (q) set("q", "")
              else e.currentTarget.blur()
            }
            if (e.key === "ArrowDown") {
              e.preventDefault()
              e.currentTarget.blur()
              setCursor(0)
            }
            if (e.key === "Enter" && items[0]) {
              e.preventDefault()
              navigate(`/contacts/${items[cursor >= 0 ? cursor : 0].id}${suffix}`)
            }
          }}
          placeholder={t`Search name, e-mail or id…`}
          aria-label={t`Search contacts`}
          className="min-w-0 flex-1 bg-transparent text-small outline-none placeholder:text-faint"
          data-testid="contacts-search"
        />
        {q && (
          <button
            type="button"
            onClick={() => set("q", "")}
            className="grid size-6 shrink-0 place-items-center rounded-lg text-faint hover:bg-muted hover:text-foreground"
            aria-label={t`Clear the search`}
          >
            <XIcon className="size-3.5" />
          </button>
        )}
      </label>
      {!global && (inboxes?.length ?? 0) > 1 && (
        <FilterChip
          value={chosen}
          onChange={(id) => set("inbox", id)}
          options={(inboxes ?? []).map((i) => ({ value: i.id, label: i.name }))}
          placeholder={<Trans>Product</Trans>}
          anyLabel={<Trans>Every product</Trans>}
          ariaLabel={t`Filter by product`}
          testId="contacts-product"
        />
      )}
      {(countries.length > 0 || country) && (
        <FilterChip
          value={country}
          onChange={(v) => set("country", v)}
          options={countries.map((x) => ({ value: x, label: x }))}
          placeholder={<Trans>Country</Trans>}
          anyLabel={<Trans>Any country</Trans>}
          ariaLabel={t`Filter by country`}
          testId="contacts-country"
        />
      )}
      <ToggleChip on={blockedOnly} onClick={() => set("blocked", blockedOnly ? "" : "1")} testId="contacts-blocked">
        <Trans>Blocked</Trans>
      </ToggleChip>
      <ToggleChip
        on={undeliverableOnly}
        onClick={() => set("undeliverable", undeliverableOnly ? "" : "1")}
        testId="contacts-undeliverable"
      >
        <Trans>Undeliverable</Trans>
      </ToggleChip>
      {anyFilter && (
        <button
          type="button"
          onClick={clearFilters}
          className="h-7 rounded-[7px] px-2 text-small font-medium outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
          data-testid="contacts-clear"
        >
          <Trans>Clear filters</Trans>
        </button>
      )}
      <span className="ms-auto text-caption text-faint tabular-nums" data-testid="contacts-count">
        {directory.hasNextPage ? <Trans>People so far: {shownCount}</Trans> : <Trans>People: {shownCount}</Trans>}
      </span>
      {canManage && items.length > 0 && !selection.selecting && <SelectToggle selection={selection} />}
    </div>
  )

  const table = (
    <div className="min-h-0 flex-1 overflow-auto px-4 pb-6">
      <div
        className={cn(
          GRID,
          "sticky top-0 z-[2] h-[34px] border-b bg-surface text-[11px] font-semibold tracking-[0.05em] text-faint uppercase"
        )}
        aria-hidden
      >
        <span>
          <Trans>Person</Trans>
        </span>
        <span>
          <Trans>Products</Trans>
        </span>
        <span>
          <Trans>Country</Trans>
        </span>
        <span>
          <Trans>Convers.</Trans>
        </span>
        <span className="max-xl:hidden">
          <Trans>Last seen</Trans>
        </span>
      </div>
      {directory.isPending ? (
        <div className="flex flex-col" aria-busy="true">
          {[62, 48, 70, 54, 66, 44, 58, 50].map((w, i) => (
            <div key={i} className="flex h-14 items-center gap-3 border-b border-b-muted px-3">
              <Skeleton className="size-[34px] shrink-0 rounded-full" />
              <div className="flex flex-1 flex-col gap-1.5">
                <Skeleton className="h-3" style={{ width: `${w}%`, maxWidth: 240 }} />
                <Skeleton className="h-2.5 w-30" />
              </div>
            </div>
          ))}
        </div>
      ) : directory.error ? (
        <ErrorLine error={directory.error} className="p-4" />
      ) : items.length === 0 ? (
        <div className="mx-auto flex max-w-sm flex-col items-center gap-3 px-2 py-14 text-center">
          <span className="grid size-12 place-items-center rounded-xl bg-brand-wash text-brand">
            <BookUserIcon className="size-5" aria-hidden />
          </span>
          <p className="text-body text-faint">
            {query ? (
              <Trans>No contact matches “{query}”.</Trans>
            ) : anyFilter ? (
              <Trans>No contacts in this view.</Trans>
            ) : (
              <Trans>No contacts yet. They appear when someone writes in.</Trans>
            )}
          </p>
          {anyFilter ? (
            <Button variant="outline" className={actionOutline} onClick={clearFilters}>
              <Trans>Clear filters</Trans>
            </Button>
          ) : (
            <Button variant="outline" className={actionOutline} onClick={() => setAdding(true)}>
              <Trans>Add contact</Trans>
            </Button>
          )}
        </div>
      ) : (
        <ul ref={list} className="flex flex-col">
          {items.map((c, i) => (
            <Row
              key={c.id}
              c={c}
              current={c.id === shown}
              cursor={i === cursor}
              search={suffix}
              check={
                canManage && selection.selecting
                  ? { checked: selection.has(c.id), onToggle: (range) => selection.toggle(c.id, range) }
                  : undefined
              }
            />
          ))}
        </ul>
      )}
      {directory.hasNextPage && (
        <div className="flex justify-center py-4">
          <Button
            variant="outline"
            className={actionOutline}
            onClick={() => void directory.fetchNextPage()}
            disabled={directory.isFetchingNextPage}
            data-testid="contacts-more"
          >
            <Trans>Load more</Trans>
          </Button>
        </div>
      )}
    </div>
  )

  return (
    <main className="flex h-svh min-h-0 flex-col bg-surface max-md:h-[calc(100svh-49px)]" data-testid="contacts-page">
      {tab === "duplicates" ? (
        <>
          {header}
          {duplicates}
        </>
      ) : fullPage ? (
        <>
          {header}
          <div className="min-h-0 flex-1 overflow-y-auto px-6 pt-4 pb-16">
            <ContactDetail
              key={contactId}
              id={contactId}
              page
              back={{ to: `/contacts/${contactId}${suffix}`, label: t`Contacts` }}
            />
          </div>
        </>
      ) : (
        <div
          className={cn(
            "grid min-h-0 flex-1",
            shown ? "grid-cols-[minmax(0,1fr)_440px] max-xl:grid-cols-[minmax(0,1fr)_380px]" : "grid-cols-1"
          )}
        >
          <section className="flex min-h-0 min-w-0 flex-col" aria-label={t`Contacts`}>
            {header}
            {toolbar}
            {canManage && <ContactBulkBar items={items} selection={selection} className="px-6 pb-2" />}
            {table}
          </section>
          {shown && (
            <aside className="flex min-h-0 flex-col overflow-hidden border-s bg-card" aria-label={t`Contact details`}>
              <ContactDetail
                key={shown}
                id={shown}
                page={false}
                pageLink={`/contacts/${shown}${suffix}${suffix ? "&" : "?"}view=page`}
              />
            </aside>
          )}
        </div>
      )}
      <AddContactDialog open={adding} onOpenChange={setAdding} />
    </main>
  )
}
