import { Trans, useLingui } from "@lingui/react/macro"
import { ChevronDownIcon, PlusIcon, SearchIcon, XIcon } from "lucide-react"
import { useEffect, useMemo, useRef, useState } from "react"
import { Link, useNavigate, useSearchParams } from "react-router"

import { ContactAvatar, ErrorLine, toast } from "@/components/common"
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
import { Ago, look } from "@/features/inbox/PageFrame"
import { ProductTag } from "@/features/inbox/ProductTag"
import { headButton, Page, PageHeader, PageTabs } from "@/features/settings/ui"
import { useDebounced } from "@/hooks/use-debounced"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useSelection } from "@/hooks/use-selection"
import type { Contact } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

type Tab = "all" | "recent" | "duplicates"

const GRID =
  "grid grid-cols-[minmax(0,1.6fr)_minmax(0,1fr)_64px_92px_minmax(0,90px)] items-center gap-3 px-4 max-xl:grid-cols-[minmax(0,1.6fr)_minmax(0,1fr)_64px_92px]"

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
        render={<button type="button" className={current ? look.chipOn : look.chipDash} />}
        aria-label={ariaLabel}
        data-testid={testId}
      >
        {current ? (
          <>
            <span className="max-w-28 truncate">{current.label}</span>
            <ChevronDownIcon className="size-3.5" />
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
      className={on ? look.chipOn : look.chipDash}
      data-testid={testId}
    >
      {children}
    </button>
  )
}

function Pill({ tone, children }: { tone: "danger" | "warn"; children: React.ReactNode }) {
  return (
    <span
      className={cn(
        "shrink-0 rounded-full border px-1.5 text-[11px] leading-4 font-medium",
        tone === "danger" ? "border-destructive/35 text-destructive" : "border-warning/45 text-warning"
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
  const name = contactName(c, t`Unnamed contact`)
  const sub = c.emails[0] && c.emails[0] !== name ? c.emails[0] : (c.external_ids[0]?.external_id ?? "")
  const seen = c.activity?.last_seen_at
  return (
    <li className="flex items-center border-t">
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
          "py-2.5 outline-none transition-colors hover:bg-surface focus-visible:bg-surface",
          current && "bg-surface shadow-[inset_2px_0_0_var(--brand)] rtl:shadow-[inset_-2px_0_0_var(--brand)]",
          cursor && !current && "bg-surface"
        )}
        aria-current={current ? "page" : undefined}
        data-testid="contact-row"
      >
        <span className="flex min-w-0 items-center gap-2.5">
          <ContactAvatar id={c.id} name={name} className="size-[30px] shrink-0 text-caption font-semibold" />
          <span className="flex min-w-0 flex-col leading-[1.3]">
            <span className="flex min-w-0 items-center gap-1.5">
              <span className="truncate font-medium">{name}</span>
              {c.blocked && (
                <Pill tone="danger">
                  <Trans>Blocked</Trans>
                </Pill>
              )}
              {c.undeliverable.length > 0 && (
                <Pill tone="warn">
                  <Trans>Undeliverable</Trans>
                </Pill>
              )}
            </span>
            <span className="truncate text-caption text-faint">{sub || <Trans>Visitor</Trans>}</span>
          </span>
        </span>
        <span className="flex min-w-0 flex-wrap gap-1 overflow-hidden">
          {c.inboxes.slice(0, 3).map((i) => (
            <ProductTag key={i.id} id={i.id} always />
          ))}
        </span>
        <span className="tabular-nums">
          {c.activity ? new Intl.NumberFormat(i18n.locale).format(c.activity.conversations) : "—"}
        </span>
        <span className="truncate">{seen ? <Ago iso={seen} /> : <span className="text-faint">—</span>}</span>
        <span className="truncate max-xl:hidden">
          {countryOf(c, i18n.locale) ?? <span className="text-faint">—</span>}
        </span>
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
    <div className="flex min-w-0 flex-1 items-center gap-3 px-4 py-4">
      <ContactAvatar id={c.id} name={name} className="size-10 shrink-0 font-semibold" />
      <span className="flex min-w-0 flex-col leading-[1.35]">
        <Link to={`/contacts/${c.id}`} className="truncate font-semibold hover:underline">
          {name}
        </Link>
        <span className="truncate text-caption text-muted-foreground">{c.emails[0] ?? <Trans>no e-mail</Trans>}</span>
        <span className="truncate text-caption text-muted-foreground">
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
      className="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_220px] overflow-hidden rounded-xl border phone:grid-cols-1"
      data-testid="duplicate-pair"
    >
      <Side c={pair.a} />
      <div className="my-3 flex min-w-0 border-s border-dashed phone:my-0 phone:border-s-0 phone:border-t">
        <Side c={pair.b} />
      </div>
      <div className="flex flex-col justify-center gap-2.5 border-s px-4 py-3.5 phone:border-s-0 phone:border-t">
        <p className="text-caption text-muted-foreground">
          {shared ? <Trans>Same name, both wrote to {shared}.</Trans> : <Trans>Same name.</Trans>}
        </p>
        <div className="flex gap-2">
          {canManage && (
            <Button className={look.btnPSm} onClick={() => setMerging(true)} data-testid="duplicate-merge">
              <Trans>Merge…</Trans>
            </Button>
          )}
          <Button variant="outline" className={look.btnSm} onClick={onDismiss} data-testid="duplicate-dismiss">
            <Trans>Not the same</Trans>
          </Button>
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
              className="h-9"
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
              className="h-9"
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
    <PageHeader
      back={false}
      title={<Trans>Contacts</Trans>}
      description={
        product ? (
          <Trans>People who wrote to {product}</Trans>
        ) : (
          <Trans>Everyone who has written to your products</Trans>
        )
      }
      action={
        <Button className={headButton.primary} onClick={() => setAdding(true)} data-testid="add-contact-open">
          <Trans>Add contact</Trans>
        </Button>
      }
      tabs={
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
      }
    />
  )

  const duplicates = (
    <div className="flex flex-col gap-3" data-testid="duplicates">
      <p className="max-w-3xl text-muted-foreground">
        <Trans>
          These look like the same person. Merging keeps the left one and moves addresses, ids and conversations into
          it. Nothing is merged until you have seen what changes.
        </Trans>
      </p>
      {everyone.isPending ? (
        <Skeleton className="h-24 w-full max-w-[980px] rounded-xl" />
      ) : pairs.length === 0 ? (
        <p className="max-w-[980px] rounded-xl border border-dashed px-4 py-10 text-center text-faint">
          <Trans>No contacts look alike right now.</Trans>
        </p>
      ) : (
        <ul className="flex max-w-[980px] flex-col gap-4">
          {pairs.map((p) => (
            <PairCard key={p.key} pair={p} onDismiss={() => dismiss(p.key)} />
          ))}
        </ul>
      )}
      {everyone.hasNextPage && (
        <Button
          variant="ghost"
          size="sm"
          className="self-start"
          onClick={() => void everyone.fetchNextPage()}
          disabled={everyone.isFetchingNextPage}
        >
          <Trans>Look through more contacts</Trans>
        </Button>
      )}
    </div>
  )

  const toolbar = (
    <div className="flex flex-wrap items-center gap-2" data-testid="contacts-filters">
      <label className="relative flex h-[34px] w-[280px] max-w-full items-center">
        <SearchIcon className="pointer-events-none absolute start-2.5 size-3.5 text-faint" aria-hidden />
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
          className="size-full rounded-lg border border-input bg-background ps-8 pe-8 text-body outline-none placeholder:text-faint focus-visible:border-faint"
          data-testid="contacts-search"
        />
        {q && (
          <button
            type="button"
            onClick={() => set("q", "")}
            className="absolute end-2 text-faint hover:text-foreground"
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
        <Button variant="ghost" size="sm" onClick={clearFilters} data-testid="contacts-clear">
          <Trans>Clear filters</Trans>
        </Button>
      )}
      <span className="ms-auto text-caption text-faint tabular-nums" data-testid="contacts-count">
        {directory.hasNextPage ? <Trans>People so far: {shownCount}</Trans> : <Trans>People: {shownCount}</Trans>}
      </span>
      {canManage && items.length > 0 && !selection.selecting && <SelectToggle selection={selection} />}
    </div>
  )

  const table = (
    <div className={look.card}>
      <div className={cn(GRID, "h-[38px]", look.th)} aria-hidden>
        <span>
          <Trans>Person</Trans>
        </span>
        <span>
          <Trans>Products</Trans>
        </span>
        <span>
          <Trans>Convers.</Trans>
        </span>
        <span>
          <Trans>Last seen</Trans>
        </span>
        <span className="max-xl:hidden">
          <Trans>Country</Trans>
        </span>
      </div>
      {directory.isPending ? (
        <div className="flex flex-col gap-2 border-t p-3">
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-full" />
        </div>
      ) : directory.error ? (
        <ErrorLine error={directory.error} className="border-t p-4" />
      ) : items.length === 0 ? (
        <p className="border-t px-4 py-10 text-center text-faint">
          {query ? (
            <Trans>No contact matches “{query}”.</Trans>
          ) : anyFilter ? (
            <Trans>No contacts in this view.</Trans>
          ) : (
            <Trans>No contacts yet. They appear when someone writes in.</Trans>
          )}
        </p>
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
        <div className="flex justify-center border-t py-2">
          <Button
            variant="ghost"
            size="sm"
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
    <Page testId="contacts-page">
      {header}
      {tab === "duplicates" ? (
        duplicates
      ) : fullPage ? (
        <ContactDetail
          key={contactId}
          id={contactId}
          page
          back={{ to: `/contacts/${contactId}${suffix}`, label: t`Contacts` }}
        />
      ) : (
        <div className="grid grid-cols-[minmax(0,1fr)_400px] items-start gap-6 max-xl:grid-cols-[minmax(0,1fr)_360px]">
          <div className="flex min-w-0 flex-col gap-3">
            {toolbar}
            {canManage && <ContactBulkBar items={items} selection={selection} />}
            {table}
          </div>
          {shown && (
            <aside
              className="sticky top-48 max-h-[calc(100svh-13rem)] overflow-y-auto rounded-xl border bg-card"
              aria-label={t`Contact details`}
            >
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
    </Page>
  )
}
