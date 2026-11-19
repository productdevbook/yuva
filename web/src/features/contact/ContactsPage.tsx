import { plural } from "@lingui/core/macro"
import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowLeftRightIcon, ChevronDownIcon, PlusIcon, SearchIcon, XIcon } from "lucide-react"
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react"
import { Link, useNavigate, useSearchParams } from "react-router"

import {
  ContactAvatar,
  Dot,
  ErrorLine,
  FlatEmpty,
  InfoColumn,
  InfoSection,
  KV,
  NavGroup,
  NavList,
  PageHeading,
  PageLayout,
  PageTop,
  toast,
  TopSep,
  UnreadBadge,
} from "@/components/common"
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
import { ContactDetail, ContactInfo } from "@/features/contact/ContactDetail"
import { MergeContactDialog } from "@/features/contact/MergeContactDialog"
import { useContactDirectory, useCreateContact } from "@/features/contact/queries"
import { contactName } from "@/features/contact/Section"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { Ago } from "@/features/inbox/PageFrame"
import { useDebounced } from "@/hooks/use-debounced"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useSelection } from "@/hooks/use-selection"
import type { Contact } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

type Tab = "all" | "recent" | "duplicates"

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
  onOpen,
  check,
}: {
  c: Contact
  current: boolean
  cursor: boolean
  search: string
  onOpen: () => void
  check?: { checked: boolean; onToggle: (range: boolean) => void }
}) {
  const { t, i18n } = useLingui()
  const inboxes = useInboxes().data
  const name = contactName(c, t`Unnamed contact`)
  const sub = c.emails[0] && c.emails[0] !== name ? c.emails[0] : (c.external_ids[0]?.external_id ?? "")
  const seen = c.activity?.last_seen_at
  const open = c.activity?.open_conversations ?? 0
  return (
    <li
      className={cn(
        "relative flex scroll-mt-3 scroll-mb-6 items-center border-t border-line-soft transition-colors hover:bg-subtle",
        (current || cursor) && "bg-subtle shadow-[inset_2px_0_0_var(--brand)] rtl:shadow-[inset_-2px_0_0_var(--brand)]",
        check?.checked && "bg-brand-wash"
      )}
    >
      {check && <RowCheck className="ms-1" checked={check.checked} name={name} onToggle={check.onToggle} />}
      <Link
        to={`/contacts/${c.id}${search}`}
        onClick={(e) => {
          if (!check) return onOpen()
          e.preventDefault()
          check.onToggle(e.shiftKey)
        }}
        className="flex min-w-0 flex-1 items-center gap-3.5 px-1 py-[11px] text-body text-fg2 outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset"
        aria-current={current ? "page" : undefined}
        data-testid="contact-row"
      >
        <ContactAvatar id={c.id} name={name} className="size-7 shrink-0 text-[11px] font-semibold" />
        <span className="flex min-w-0 flex-1 flex-col">
          <span className="flex min-w-0 items-center gap-1.5">
            <span className="truncate font-medium text-foreground">{name}</span>
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
          <span className="truncate font-mono text-xs text-muted-foreground">{sub || <Trans>Visitor</Trans>}</span>
        </span>
        <span className="flex w-40 min-w-0 shrink-0 items-center gap-1.5 @max-2xl/list:hidden">
          {c.inboxes.slice(0, 4).map((i) => (
            <span key={i.id} title={i.name} className="inline-flex">
              <Dot color={inboxes?.find((x) => x.id === i.id)?.branding.color} className="size-[7px] rounded-full" />
            </span>
          ))}
          {c.inboxes.length === 1 && (
            <span className="min-w-0 truncate text-small text-muted-foreground">{c.inboxes[0].name}</span>
          )}
        </span>
        <span className="w-24 shrink-0 truncate text-small text-muted-foreground @max-3xl/list:hidden">
          {countryOf(c, i18n.locale) ?? "—"}
        </span>
        <span className="w-20 shrink-0 truncate font-mono text-xs text-muted2">{seen ? <Ago iso={seen} /> : "—"}</span>
        <span className="flex w-7 shrink-0 justify-end">
          {open > 0 && <UnreadBadge label={plural(open, { one: "# open", other: "# open" })}>{open}</UnreadBadge>}
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
      className="grid grid-cols-[minmax(0,1fr)_40px_minmax(0,1fr)_auto] items-center gap-3 border-t border-line-soft px-1 py-3.5 @max-3xl/list:grid-cols-[minmax(0,1fr)_40px_minmax(0,1fr)] @max-3xl/list:[&>:last-child]:col-span-full"
      data-testid="duplicate-pair"
    >
      <Side c={pair.a} />
      <ArrowLeftRightIcon className="mx-auto size-4 text-faint" aria-hidden />
      <Side c={pair.b} />
      <div className="flex flex-col items-end gap-1.5 @max-3xl/list:items-start">
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

let listY = 0

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
  const openOnly = params.get("open") === "1"
  const blockedOnly = params.get("blocked") === "1"
  const undeliverableOnly = params.get("undeliverable") === "1"
  const query = useDebounced(q.trim(), 250)
  const directory = useContactDirectory(
    query,
    openOnly ? "open" : "all",
    inboxId,
    tab === "recent" ? "last_seen" : "created"
  )
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
    `${tab}|${query}|${inboxId}|${country}|${openOnly}|${blockedOnly}|${undeliverableOnly}`
  )
  const list = useRef<HTMLUListElement>(null)
  const search = useRef<HTMLInputElement>(null)
  const listParams = new URLSearchParams(params)
  listParams.delete("view")
  const suffix = listParams.toString() ? `?${listParams.toString()}` : ""
  const anyFilter = !!(q || chosen || country || openOnly || blockedOnly || undeliverableOnly)
  const shownCount = new Intl.NumberFormat(i18n.locale).format(items.length)
  const viewName = tab === "recent" ? t`Recently active` : tab === "duplicates" ? t`Duplicates` : t`People`
  const pair = contactId ? pairs.find((p) => p.a.id === contactId || p.b.id === contactId) : undefined
  const listPath = `/contacts${suffix}`

  useLayoutEffect(() => {
    window.scrollTo({ top: contactId ? 0 : listY })
  }, [contactId])
  const open = (id: string) => {
    listY = window.scrollY
    navigate(`/contacts/${id}${suffix}`)
  }

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
    return { pathname: "/contacts", search: next.toString() ? `?${next.toString()}` : "" }
  }
  const clearFilters = () =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev)
        for (const k of ["q", "inbox", "country", "open", "blocked", "undeliverable"]) next.delete(k)
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
    contactId
      ? { Escape: () => navigate(listPath) }
      : tab === "duplicates"
        ? {}
        : {
            j: () => setCursor((i) => Math.min(items.length - 1, i + 1)),
            k: () => setCursor((i) => Math.max(0, i - 1)),
            ArrowDown: () => setCursor((i) => Math.min(items.length - 1, i + 1)),
            ArrowUp: () => setCursor((i) => Math.max(0, i - 1)),
            Enter: () => cursor >= 0 && items[cursor] && open(items[cursor].id),
            o: () => cursor >= 0 && items[cursor] && open(items[cursor].id),
            "/": () => search.current?.focus(),
            ...(canManage
              ? {
                  x: () => {
                    const id = items[cursor]?.id
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

  const nav = (
    <NavList label={t`Contacts`}>
      <NavGroup
        items={[
          { key: "all", label: t`People`, to: tabTo("all"), current: tab === "all", testId: "contacts-filter-all" },
          {
            key: "recent",
            label: t`Recently active`,
            to: tabTo("recent"),
            current: tab === "recent",
            testId: "contacts-filter-recent",
          },
          {
            key: "duplicates",
            label: t`Duplicates`,
            to: tabTo("duplicates"),
            current: tab === "duplicates",
            count: everyone.data && !everyone.hasNextPage ? pairs.length : undefined,
            testId: "contacts-filter-duplicates",
          },
        ]}
      />
    </NavList>
  )

  const heading = (
    <div data-testid="contacts-head">
      <PageHeading
        title={viewName}
        lede={
          tab === "duplicates" ? (
            <Trans>
              These look like the same person. Merging keeps the left one and moves addresses, ids and conversations
              into it. Nothing is merged until you have seen what changes.
            </Trans>
          ) : product ? (
            <Trans>People who wrote to {product}</Trans>
          ) : (
            <Trans>Everyone who has written to your products</Trans>
          )
        }
      />
    </div>
  )

  const duplicates = (
    <div className="mt-7 flex flex-col gap-3" data-testid="duplicates">
      {everyone.isPending ? (
        <Skeleton className="h-16 w-full" />
      ) : pairs.length === 0 ? (
        <FlatEmpty>
          <p className="m-0">
            <Trans>No contacts look alike right now.</Trans>
          </p>
        </FlatEmpty>
      ) : (
        <ul className="m-0 flex list-none flex-col p-0">
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
    <div className="mt-5 flex flex-wrap items-center gap-2" data-testid="contacts-filters">
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
              open(items[cursor >= 0 ? cursor : 0].id)
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
      <ToggleChip on={openOnly} onClick={() => set("open", openOnly ? "" : "1")} testId="contacts-open">
        <Trans>Has an open conversation</Trans>
      </ToggleChip>
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
      <span className="ms-auto text-caption text-muted-foreground tabular-nums" data-testid="contacts-count">
        {directory.hasNextPage ? <Trans>People so far: {shownCount}</Trans> : <Trans>People: {shownCount}</Trans>}
      </span>
      {canManage && items.length > 0 && !selection.selecting && <SelectToggle selection={selection} />}
    </div>
  )

  const table = (
    <div className="mt-7">
      <div
        className="flex items-center gap-3.5 px-1 pb-2 text-caption font-medium text-muted2"
        aria-hidden
        data-testid="contacts-columns"
      >
        <span className="w-7 shrink-0" />
        <span className="min-w-0 flex-1">
          <Trans>Person</Trans>
        </span>
        <span className="w-40 shrink-0 @max-2xl/list:hidden">
          <Trans>Products</Trans>
        </span>
        <span className="w-24 shrink-0 @max-3xl/list:hidden">
          <Trans>Country</Trans>
        </span>
        <span className="w-20 shrink-0">
          <Trans>Last seen</Trans>
        </span>
        <span className="w-7 shrink-0 text-end">
          <Trans>Open</Trans>
        </span>
      </div>
      {directory.isPending ? (
        <div className="flex flex-col gap-3 border-t border-line-soft pt-3" aria-busy="true">
          {[62, 48, 70, 54, 66].map((w, i) => (
            <Skeleton key={i} className="h-5" style={{ width: `${w}%` }} />
          ))}
        </div>
      ) : directory.error ? (
        <ErrorLine error={directory.error} className="py-4" />
      ) : items.length === 0 ? (
        <FlatEmpty>
          <p className="m-0">
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
        </FlatEmpty>
      ) : (
        <ul ref={list} className="m-0 flex list-none flex-col p-0">
          {items.map((c, i) => (
            <Row
              key={c.id}
              c={c}
              current={false}
              cursor={i === cursor}
              search={suffix}
              onOpen={() => {
                listY = window.scrollY
              }}
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
        <div className="flex border-t border-line-soft pt-4">
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

  const withOpen = items.filter((c) => (c.activity?.open_conversations ?? 0) > 0).length
  const fmt = new Intl.NumberFormat(i18n.locale)

  return (
    <PageLayout
      testId="contacts-page"
      label={t`Contacts`}
      nav={nav}
      aside={
        <InfoColumn label={t`Details`}>
          {contactId ? (
            <ContactInfo
              key={contactId}
              id={contactId}
              duplicate={pair ? (pair.a.id === contactId ? pair.b : pair.a) : undefined}
              onNotSame={pair ? () => dismiss(pair.key) : undefined}
            />
          ) : (
            <InfoSection title={viewName} testId="contacts-summary">
              <KV label={t`People`} value={`${shownCount}${directory.hasNextPage ? "+" : ""}`} />
              <KV
                label={t`Has an open conversation`}
                value={fmt.format(withOpen)}
                tone={withOpen > 0 ? "brand" : undefined}
              />
              {everyone.data && !everyone.hasNextPage && (
                <KV label={t`Duplicates`} value={fmt.format(pairs.length)} to={tabTo("duplicates")} />
              )}
            </InfoSection>
          )}
        </InfoColumn>
      }
    >
      <PageTop
        back={contactId ? { label: viewName, to: listPath } : undefined}
        actions={
          <Button className={actionInk} onClick={() => setAdding(true)} data-testid="add-contact-open">
            <PlusIcon className="size-3.5" aria-hidden />
            <span className={contactId ? "sr-only" : "max-lg:sr-only"}>
              <Trans>Add contact</Trans>
            </span>
          </Button>
        }
      >
        <span className="font-medium text-foreground">
          <Trans>Contacts</Trans>
        </span>
        <TopSep />
        <span className="truncate">{viewName}</span>
        <TopSep />
        <span className="truncate">{product ?? t`All products`}</span>
      </PageTop>
      {contactId ? (
        <ContactDetail key={contactId} id={contactId} page={false} />
      ) : (
        <section aria-label={viewName} className="@container/list max-w-[860px]">
          {heading}
          {tab === "duplicates" ? (
            duplicates
          ) : (
            <>
              {toolbar}
              {canManage && <ContactBulkBar items={items} selection={selection} className="mt-3" />}
              {table}
            </>
          )}
        </section>
      )}
      <AddContactDialog open={adding} onOpenChange={setAdding} />
    </PageLayout>
  )
}
