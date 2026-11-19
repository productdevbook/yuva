import { plural } from "@lingui/core/macro"
import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowLeftRightIcon, ChevronDownIcon, PlusIcon, SearchIcon, XIcon } from "lucide-react"
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react"
import { Link, useNavigate, useSearchParams } from "react-router"

import { ContactAvatar, Dot, ErrorLine, InfoColumn, Kbd, PageLayout, PageTop, toast } from "@/components/common"
import { FormCard, FormGrid } from "@/components/common/FormCard"
import {
  filterButton,
  filterButtonOn,
  FilterRow,
  KeyHints,
  ListBox,
  ListBoxEmpty,
  ListBoxSkeleton,
  PageTitle,
  RowCheckbox,
} from "@/components/common/ListBox"
import { SHORTCUTS } from "@/components/common/ShortcutSheet"
import { useErrorText } from "@/components/common/text"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Field, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from "@/components/ui/input-group"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { ContactBulkBar } from "@/features/contact/ContactBulk"
import { ContactDetail, ContactInfo } from "@/features/contact/ContactDetail"
import { MergeContactDialog } from "@/features/contact/MergeContactDialog"
import { useContactDirectory, useCreateContact, type DirectoryFilter } from "@/features/contact/queries"
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

export const PEOPLE_VIEWS = ["all", "customers", "visitors", "recent", "duplicates"] as const
export type PeopleView = (typeof PEOPLE_VIEWS)[number]

export function viewOf(params: URLSearchParams): PeopleView {
  const v = params.get("tab")
  return PEOPLE_VIEWS.find((x) => x === v) ?? "all"
}

export function usePeopleViewNames(): Record<PeopleView, string> {
  const { t } = useLingui()
  return {
    all: t`All people`,
    customers: t`Customers`,
    visitors: t`Visitors`,
    recent: t`Recently active`,
    duplicates: t`Duplicates`,
  }
}

const FILTERS = ["q", "inbox", "plan", "country", "open", "blocked", "undeliverable"]

function attr(c: Contact, key: string) {
  const v = c.attributes?.[key]
  return typeof v === "string" && v.trim() ? v.trim() : undefined
}

export function countryOf(c: Contact, locale: string) {
  const own = attr(c, "country")
  if (own) return own
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

function usePeopleScope() {
  const [params] = useSearchParams()
  const [global] = useInboxFilter()
  const inboxes = useInboxes().data
  const picked = params.get("inbox") ?? ""
  const chosen = inboxes?.some((i) => i.id === picked) ? picked : ""
  return { chosen, inboxId: chosen || global, global }
}

export function useDuplicatePairs(dismissed: string[] = readDismissed()) {
  const { inboxId } = usePeopleScope()
  const everyone = useContactDirectory("", {}, inboxId, "last_seen")
  const all = useMemo(() => everyone.data?.pages.flatMap((p) => p.items) ?? [], [everyone.data])
  const pairs = useDuplicates(all, dismissed)
  return { everyone, pairs, complete: !!everyone.data && !everyone.hasNextPage }
}

function FilterMenu({
  value,
  onChange,
  options,
  anyLabel,
  label,
  testId,
}: {
  value: string
  onChange: (v: string) => void
  options: { value: string; label: string }[]
  anyLabel: string
  label: string
  testId: string
}) {
  const current = options.find((o) => o.value === value)
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button variant="outline" size="sm" className={cn(filterButton, "max-w-52", current && filterButtonOn)} />
        }
        aria-label={label}
        data-testid={testId}
      >
        <span className="truncate">{current?.label ?? anyLabel}</span>
        <ChevronDownIcon className="text-muted-foreground" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="max-h-[60svh] min-w-44">
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

function Tag({ tone, children }: { tone: "danger" | "warn"; children: React.ReactNode }) {
  return (
    <Badge
      variant="secondary"
      className={cn(
        "h-5 rounded-md px-[7px] text-[11px] font-semibold",
        tone === "danger" ? "bg-destructive/10 text-destructive" : "bg-warning/10 text-warning"
      )}
    >
      {children}
    </Badge>
  )
}

const head = "h-9 px-3 text-start text-caption font-medium text-muted-foreground"
const cell = "px-3 py-2.5 text-body"
const hideCompany = "@max-3xl/list:hidden"
const hidePlan = "@max-2xl/list:hidden"
const hideProducts = "@max-5xl/list:hidden"
const hideCount = "@max-md/list:hidden"

function Row({
  c,
  cursor,
  checked,
  selecting,
  check,
  to,
  onOpen,
}: {
  c: Contact
  cursor: boolean
  checked: boolean
  selecting: boolean
  check?: (range: boolean) => void
  to: string
  onOpen: () => void
}) {
  const { t } = useLingui()
  const inboxes = useInboxes().data
  const name = contactName(c, t`Unnamed contact`)
  const sub = c.emails[0] && c.emails[0] !== name ? c.emails[0] : (c.external_ids[0]?.external_id ?? "")
  const seen = c.activity?.last_seen_at
  const open = c.activity?.open_conversations ?? 0
  const count = c.activity?.conversations ?? 0
  const company = attr(c, "company")
  const plan = attr(c, "plan")
  return (
    <TableRow
      onClick={(e) => {
        if (check && (e.shiftKey || selecting)) check(e.shiftKey)
        else onOpen()
      }}
      className={cn(
        "cursor-pointer border-line-soft hover:bg-subtle",
        cursor && "bg-subtle [&>td:first-child]:shadow-[inset_2px_0_0_var(--brand)]",
        checked && "bg-brand-wash hover:bg-brand-wash"
      )}
      data-testid="contact-row"
      data-checked={checked || undefined}
    >
      {check && (
        <TableCell className="w-10 py-2.5 ps-3 pe-0" onClick={(e) => e.stopPropagation()}>
          <RowCheckbox checked={checked} name={name} onToggle={check} className="mt-0" />
        </TableCell>
      )}
      <TableCell className={cn(cell, "min-w-0 overflow-hidden", !check && "ps-4")}>
        <Link
          to={to}
          onClick={(e) => {
            e.stopPropagation()
            if (check && (e.shiftKey || selecting)) {
              e.preventDefault()
              check(e.shiftKey)
            } else onOpen()
          }}
          className="flex min-w-0 items-center gap-3 rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
          data-testid="contact-open"
        >
          <ContactAvatar id={c.id} name={name} className="size-7 shrink-0 text-[11px] font-semibold" />
          <span className="flex min-w-0 flex-col leading-5">
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
            <span className="truncate text-small text-muted-foreground">{sub || <Trans>Visitor</Trans>}</span>
          </span>
        </Link>
      </TableCell>
      <TableCell className={cn(cell, hideCompany, "truncate text-fg2")}>{company ?? "–"}</TableCell>
      <TableCell className={cn(cell, hidePlan, "text-fg2")}>
        {plan ? (
          <Badge variant="secondary" className="max-w-full truncate rounded-md font-normal">
            {plan}
          </Badge>
        ) : (
          <span className="text-muted-foreground">–</span>
        )}
      </TableCell>
      <TableCell className={cn(cell, hideProducts)}>
        <span className="flex min-w-0 items-center gap-2 overflow-hidden">
          {c.inboxes.slice(0, 2).map((i) => (
            <span key={i.id} className="inline-flex min-w-0 items-center gap-1.5 text-small text-muted-foreground">
              <Dot color={inboxes?.find((x) => x.id === i.id)?.branding.color} className="size-[7px] rounded-full" />
              <span className="truncate">{i.name}</span>
            </span>
          ))}
          {c.inboxes.length > 2 && <span className="text-small text-faint">+{c.inboxes.length - 2}</span>}
        </span>
      </TableCell>
      <TableCell className={cn(cell, "text-small text-muted-foreground tabular-nums")}>
        {seen ? <Ago iso={seen} /> : "–"}
      </TableCell>
      <TableCell className={cn(cell, hideCount, "pe-4 text-end tabular-nums")}>
        <span className="inline-flex items-center gap-1.5">
          <span className={count ? "text-fg2" : "text-muted-foreground"}>{count}</span>
          {open > 0 && (
            <Badge className="h-[18px] min-w-[18px] rounded-full bg-brand px-1.5 text-[11px] text-white">
              {plural(open, { one: "# open", other: "# open" })}
            </Badge>
          )}
        </span>
      </TableCell>
    </TableRow>
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
        <Link to={`/contacts/${c.id}`} className="truncate text-body font-semibold hover:underline">
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

function PairRow({ pair, onDismiss }: { pair: Pair; onDismiss: () => void }) {
  const { canManage } = useSession()
  const [merging, setMerging] = useState(false)
  const shared = pair.inbox
  return (
    <div
      data-row
      className="@container/pair flex flex-col gap-3 border-line-soft px-4 py-3.5"
      data-testid="duplicate-pair"
    >
      <div className="grid grid-cols-[minmax(0,1fr)_32px_minmax(0,1fr)] items-center gap-3 @max-md/pair:grid-cols-1">
        <Side c={pair.a} />
        <ArrowLeftRightIcon className="mx-auto size-4 text-faint @max-md/pair:hidden" aria-hidden />
        <Side c={pair.b} />
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <p className="m-0 me-auto text-caption text-muted-foreground">
          {shared ? <Trans>Same name, both wrote to {shared}.</Trans> : <Trans>Same name.</Trans>}
        </p>
        <Button variant="outline" size="sm" onClick={onDismiss} data-testid="duplicate-dismiss">
          <Trans>Not the same</Trans>
        </Button>
        {canManage && (
          <Button size="sm" onClick={() => setMerging(true)} data-testid="duplicate-merge">
            <Trans>Merge…</Trans>
          </Button>
        )}
      </div>
      {canManage && (
        <MergeContactDialog target={pair.a} initialSource={pair.b} open={merging} onOpenChange={setMerging} />
      )}
    </div>
  )
}

function AddContactForm({ onClose }: { onClose: () => void }) {
  const navigate = useNavigate()
  const errorText = useErrorText()
  const create = useCreateContact()
  const [name, setName] = useState("")
  const [email, setEmail] = useState("")
  return (
    <FormCard
      title={<Trans>Add contact</Trans>}
      description={<Trans>Someone who has not written yet. You can start a conversation with them next.</Trans>}
      submit={<Trans>Add contact</Trans>}
      ready={!!name.trim() || !!email.trim()}
      busy={create.isPending}
      onCancel={onClose}
      onSubmit={() =>
        create.mutate(
          { name: name.trim() || undefined, emails: email.trim() ? [email.trim()] : undefined },
          {
            onSuccess: (c) => {
              onClose()
              navigate(`/contacts/${c.id}`)
              toast(<Trans>Contact added. Use New conversation to write to them.</Trans>)
            },
            onError: (err) => toast(errorText(err)),
          }
        )
      }
      testId="add-contact"
    >
      <FormGrid>
        <Field>
          <FieldLabel htmlFor="add-contact-name">
            <Trans>Name</Trans>
          </FieldLabel>
          <Input
            id="add-contact-name"
            value={name}
            autoFocus
            onChange={(e) => setName(e.target.value)}
            maxLength={200}
            className="h-9"
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="add-contact-email">
            <Trans>E-mail</Trans>
          </FieldLabel>
          <Input
            id="add-contact-email"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            className="h-9"
          />
        </Field>
      </FormGrid>
    </FormCard>
  )
}

let listY = 0

export function ContactsPage({ contactId }: { contactId?: string }) {
  const { t, i18n } = useLingui()
  const { canManage } = useSession()
  const navigate = useNavigate()
  const [params, setParams] = useSearchParams()
  const view = viewOf(params)
  const names = usePeopleViewNames()
  const inboxes = useInboxes().data
  const { chosen, inboxId, global } = usePeopleScope()
  const product = inboxes?.find((i) => i.id === inboxId)?.name
  const q = params.get("q") ?? ""
  const plan = params.get("plan") ?? ""
  const country = params.get("country") ?? ""
  const openOnly = params.get("open") === "1"
  const blockedOnly = params.get("blocked") === "1"
  const undeliverableOnly = params.get("undeliverable") === "1"
  const query = useDebounced(q.trim(), 250)
  const filter: DirectoryFilter = {
    kind: view === "customers" ? "known" : view === "visitors" ? "visitor" : undefined,
    open: openOnly,
  }
  const directory = useContactDirectory(query, filter, inboxId, view === "recent" ? "last_seen" : "created")
  const loaded = useMemo(() => directory.data?.pages.flatMap((p) => p.items) ?? [], [directory.data])
  const plans = useMemo(
    () => [...new Set(loaded.map((c) => attr(c, "plan")).filter((x): x is string => !!x))].sort(),
    [loaded]
  )
  const countries = useMemo(
    () => [...new Set(loaded.map((c) => countryOf(c, i18n.locale)).filter((x): x is string => !!x))].sort(),
    [loaded, i18n.locale]
  )
  const items = useMemo(
    () =>
      loaded.filter(
        (c) =>
          (!plan || attr(c, "plan") === plan) &&
          (!country || countryOf(c, i18n.locale) === country) &&
          (!blockedOnly || c.blocked) &&
          (!undeliverableOnly || c.undeliverable.length > 0)
      ),
    [loaded, plan, country, blockedOnly, undeliverableOnly, i18n.locale]
  )
  const [dismissed, setDismissed] = useState(readDismissed)
  const { everyone, pairs } = useDuplicatePairs(dismissed)
  const [cursor, setCursor] = useState(-1)
  const [adding, setAdding] = useState(false)
  const selection = useSelection(
    items.map((c) => c.id),
    `${view}|${query}|${inboxId}|${plan}|${country}|${openOnly}|${blockedOnly}|${undeliverableOnly}`
  )
  const list = useRef<HTMLDivElement>(null)
  const search = useRef<HTMLInputElement>(null)
  const suffix = params.toString() ? `?${params.toString()}` : ""
  const anyFilter = !!(q || chosen || plan || country || openOnly || blockedOnly || undeliverableOnly)
  const fmt = new Intl.NumberFormat(i18n.locale)
  const shown = `${fmt.format(items.length)}${directory.hasNextPage ? "+" : ""}`
  const viewName = names[view]
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
  const clearFilters = () =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev)
        for (const k of FILTERS) next.delete(k)
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
      : view === "duplicates"
        ? {}
        : {
            [SHORTCUTS.next]: () => setCursor((i) => Math.min(items.length - 1, i + 1)),
            [SHORTCUTS.previous]: () => setCursor((i) => Math.max(0, i - 1)),
            ArrowDown: () => setCursor((i) => Math.min(items.length - 1, i + 1)),
            ArrowUp: () => setCursor((i) => Math.max(0, i - 1)),
            Enter: () => cursor >= 0 && items[cursor] && open(items[cursor].id),
            o: () => cursor >= 0 && items[cursor] && open(items[cursor].id),
            [SHORTCUTS.search]: () => search.current?.focus(),
            ...(canManage
              ? {
                  [SHORTCUTS.select]: () => {
                    const id = items[cursor]?.id
                    if (id) selection.toggle(id)
                  },
                }
              : {}),
          }
  )
  useEffect(() => {
    if (cursor >= 0)
      list.current?.querySelectorAll("[data-testid=contact-row]")[cursor]?.scrollIntoView({ block: "nearest" })
  }, [cursor])

  if (contactId) {
    return (
      <PageLayout
        testId="contacts-page"
        label={t`People`}
        aside={
          <InfoColumn label={t`Details`}>
            <ContactInfo
              key={contactId}
              id={contactId}
              duplicate={pair ? (pair.a.id === contactId ? pair.b : pair.a) : undefined}
              onNotSame={pair ? () => dismiss(pair.key) : undefined}
            />
          </InfoColumn>
        }
      >
        <PageTop back={{ label: viewName, to: listPath }} />
        <div className="max-w-[860px]">
          <ContactDetail key={contactId} id={contactId} page={false} />
        </div>
      </PageLayout>
    )
  }

  const lede =
    view === "duplicates" ? (
      <Trans>These look like the same person</Trans>
    ) : view === "customers" ? (
      <Trans>People with an e-mail address or an id from your app</Trans>
    ) : view === "visitors" ? (
      <Trans>Visitors without a confirmed e-mail address or id</Trans>
    ) : view === "recent" ? (
      <Trans>Most recently seen first</Trans>
    ) : product ? (
      <Trans>People who wrote to {product}</Trans>
    ) : (
      <Trans>Everyone who has written to your products</Trans>
    )

  const heading = (
    <PageTitle
      title={viewName}
      sub={
        <span data-testid="contacts-lede">
          {lede}
          {view !== "duplicates" && directory.data && (
            <span data-testid="contacts-count">
              {" · "}
              {plural(items.length, { one: `${shown} person`, other: `${shown} people` })}
            </span>
          )}
        </span>
      }
      actions={
        <Button onClick={() => setAdding(true)} disabled={adding} data-testid="add-contact-open">
          <PlusIcon />
          <Trans>Add contact</Trans>
        </Button>
      }
      testId="contacts-head"
    />
  )

  const duplicates = (
    <>
      <p className="m-0 mt-3 max-w-[720px] text-body text-muted-foreground">
        <Trans>
          Merging keeps the left one and moves addresses, ids and conversations into it. Nothing is merged until you
          have seen what changes.
        </Trans>
      </p>
      <ListBox testId="duplicates">
        {everyone.isPending ? (
          <ListBoxSkeleton rows={3} />
        ) : pairs.length === 0 ? (
          <ListBoxEmpty>
            <Trans>No contacts look alike right now.</Trans>
          </ListBoxEmpty>
        ) : (
          pairs.map((p) => <PairRow key={p.key} pair={p} onDismiss={() => dismiss(p.key)} />)
        )}
      </ListBox>
      {everyone.hasNextPage && (
        <Button
          variant="outline"
          size="sm"
          className="mt-3"
          onClick={() => void everyone.fetchNextPage()}
          disabled={everyone.isFetchingNextPage}
        >
          <Trans>Look through more contacts</Trans>
        </Button>
      )}
    </>
  )

  const states = [
    { key: "open", on: openOnly, label: t`Has an open conversation` },
    { key: "blocked", on: blockedOnly, label: t`Blocked` },
    { key: "undeliverable", on: undeliverableOnly, label: t`Undeliverable` },
  ]
  const chosenStates = states.filter((s) => s.on)

  const filters = (
    <FilterRow testId="contacts-filters">
      <InputGroup className="h-8 min-w-48 flex-[1_1_240px] rounded-md border-input bg-card shadow-none focus-within:border-faint">
        <InputGroupAddon>
          <SearchIcon className="size-[15px] text-faint" />
        </InputGroupAddon>
        <InputGroupInput
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
          data-testid="contacts-search"
        />
        <InputGroupAddon align="inline-end">
          {q ? (
            <InputGroupButton size="icon-xs" onClick={() => set("q", "")} aria-label={t`Clear the search`}>
              <XIcon />
            </InputGroupButton>
          ) : (
            <Kbd className="phone:hidden">/</Kbd>
          )}
        </InputGroupAddon>
      </InputGroup>
      {!global && (inboxes?.length ?? 0) > 1 && (
        <FilterMenu
          value={chosen}
          onChange={(id) => set("inbox", id)}
          options={(inboxes ?? []).map((i) => ({ value: i.id, label: i.name }))}
          anyLabel={t`Every product`}
          label={t`Filter by product`}
          testId="contacts-product"
        />
      )}
      {(plans.length > 0 || plan) && (
        <FilterMenu
          value={plan}
          onChange={(v) => set("plan", v)}
          options={plans.map((x) => ({ value: x, label: x }))}
          anyLabel={t`Any plan`}
          label={t`Filter by plan`}
          testId="contacts-plan"
        />
      )}
      {(countries.length > 0 || country) && (
        <FilterMenu
          value={country}
          onChange={(v) => set("country", v)}
          options={countries.map((x) => ({ value: x, label: x }))}
          anyLabel={t`Any country`}
          label={t`Filter by country`}
          testId="contacts-country"
        />
      )}
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button
              variant="outline"
              size="sm"
              className={cn(filterButton, "max-w-60", chosenStates.length > 0 && filterButtonOn)}
            />
          }
          aria-label={t`Filter by state`}
          data-testid="contacts-state"
        >
          <span className="truncate">
            {chosenStates.length > 0 ? chosenStates.map((s) => s.label).join(", ") : t`Any status`}
          </span>
          <ChevronDownIcon className="text-muted-foreground" />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="min-w-56">
          <DropdownMenuGroup>
            {states.map((s) => (
              <DropdownMenuCheckboxItem
                key={s.key}
                checked={s.on}
                onCheckedChange={(on) => set(s.key, on ? "1" : "")}
                data-testid={`contacts-${s.key}`}
              >
                {s.label}
              </DropdownMenuCheckboxItem>
            ))}
          </DropdownMenuGroup>
        </DropdownMenuContent>
      </DropdownMenu>
      {anyFilter && (
        <Button variant="ghost" size="sm" className="h-8" onClick={clearFilters} data-testid="contacts-clear">
          <Trans>Clear filters</Trans>
        </Button>
      )}
    </FilterRow>
  )

  const check = canManage
    ? (id: string) => (range: boolean) => {
        selection.toggle(id, range)
      }
    : undefined

  const table = (
    <div ref={list}>
      <ListBox bar={canManage && <ContactBulkBar items={items} selection={selection} />} testId="contacts-list">
        {directory.isPending ? (
          <ListBoxSkeleton />
        ) : directory.error && !directory.data ? (
          <ErrorLine error={directory.error} className="px-4 py-4" />
        ) : items.length === 0 ? (
          <ListBoxEmpty
            testId="contacts-empty"
            action={
              anyFilter ? (
                <Button variant="outline" size="sm" onClick={clearFilters}>
                  <Trans>Clear filters</Trans>
                </Button>
              ) : (
                <Button variant="outline" size="sm" onClick={() => setAdding(true)}>
                  <Trans>Add contact</Trans>
                </Button>
              )
            }
          >
            {query ? (
              <Trans>No contact matches “{query}”.</Trans>
            ) : anyFilter || view !== "all" ? (
              <Trans>No contacts in this view.</Trans>
            ) : (
              <Trans>No contacts yet. They appear when someone writes in.</Trans>
            )}
          </ListBoxEmpty>
        ) : (
          <Table className="table-fixed text-body">
            <TableHeader>
              <TableRow className="border-line-soft hover:bg-transparent" data-testid="contacts-columns">
                {canManage && <TableHead className="w-10 ps-3 pe-0" aria-hidden />}
                <TableHead className={cn(head, !canManage && "ps-4")}>
                  <Trans>Person</Trans>
                </TableHead>
                <TableHead className={cn(head, hideCompany, "w-[18%]")}>
                  <Trans>Company</Trans>
                </TableHead>
                <TableHead className={cn(head, hidePlan, "w-28")}>
                  <Trans>Plan</Trans>
                </TableHead>
                <TableHead className={cn(head, hideProducts, "w-40")}>
                  <Trans>Products</Trans>
                </TableHead>
                <TableHead className={cn(head, "w-28 @max-md/list:w-24")}>
                  <Trans>Last seen</Trans>
                </TableHead>
                <TableHead className={cn(head, hideCount, "w-32 pe-4 text-end")}>
                  <Trans>Conversations</Trans>
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((c, i) => (
                <Row
                  key={c.id}
                  c={c}
                  cursor={i === cursor}
                  checked={selection.has(c.id)}
                  selecting={selection.selecting}
                  check={check?.(c.id)}
                  to={`/contacts/${c.id}${suffix}`}
                  onOpen={() => open(c.id)}
                />
              ))}
            </TableBody>
          </Table>
        )}
      </ListBox>
      <ErrorLine error={directory.data ? directory.error : null} className="py-2" />
      {directory.hasNextPage && (
        <Button
          variant="outline"
          size="sm"
          className="mt-3"
          onClick={() => void directory.fetchNextPage()}
          disabled={directory.isFetchingNextPage}
          data-testid="contacts-more"
        >
          <Trans>Load more</Trans>
        </Button>
      )}
      <KeyHints
        items={[
          [[SHORTCUTS.next, SHORTCUTS.previous], <Trans key="move">move</Trans>],
          [
            ["↵"],
            <Trans key="open" context="keyboard hint">
              open
            </Trans>,
          ],
          ...(canManage
            ? [[[SHORTCUTS.select], <Trans key="select">select</Trans>] as [string[], React.ReactNode]]
            : []),
          [[SHORTCUTS.search], <Trans key="search">search</Trans>],
        ]}
      />
    </div>
  )

  return (
    <PageLayout testId="contacts-page" label={t`People`}>
      <section aria-label={viewName} className="@container/list min-w-0">
        {heading}
        {adding && <AddContactForm onClose={() => setAdding(false)} />}
        {view === "duplicates" ? (
          duplicates
        ) : (
          <>
            {filters}
            {table}
          </>
        )}
      </section>
    </PageLayout>
  )
}
