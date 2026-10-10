import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { BookUserIcon, SearchIcon, XIcon } from "lucide-react"
import { useEffect, useRef, useState } from "react"
import { Link, useMatch, useNavigate } from "react-router"

import { ContactAvatar, ErrorLine } from "@/components/common"
import { PaneEmpty } from "@/components/common/Column"
import { formatDateTime, formatShort } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from "@/components/ui/input-group"
import { Skeleton } from "@/components/ui/skeleton"
import { useContactDirectory, type DirectoryFilter } from "@/features/contact/queries"
import { contactName } from "@/features/contact/Section"
import { useHotkeys } from "@/hooks/use-hotkeys"
import type { Contact } from "@/lib/api"
import { cn } from "@/lib/utils"

function useDebounced<T>(value: T, ms: number) {
  const [v, setV] = useState(value)
  useEffect(() => {
    const id = setTimeout(() => setV(value), ms)
    return () => clearTimeout(id)
  }, [value, ms])
  return v
}

function Row({ c, current, cursor }: { c: Contact; current: boolean; cursor: boolean }) {
  const { t, i18n } = useLingui()
  const name = contactName(c, t`Unnamed contact`)
  const known = c.emails.length > 0 || c.external_ids.length > 0
  const last = c.activity?.last_conversation_at
  return (
    <li>
      <Link
        to={`/contacts/${c.id}`}
        className={cn(
          "flex items-center gap-3 rounded-xl px-2.5 py-2.5 outline-none transition-colors hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring",
          current && "bg-muted",
          cursor && "ring-2 ring-ring",
        )}
        aria-current={current ? "page" : undefined}
        data-testid="contact-row"
      >
        <ContactAvatar id={c.id} name={name} className="size-10 shrink-0" />
        <span className="min-w-0 flex-1">
          <span className="flex items-center gap-2">
            <span className="min-w-0 flex-1 truncate text-body font-medium">{name}</span>
            {last && (
              <time className="shrink-0 text-caption text-faint" dateTime={last} title={formatDateTime(last, i18n.locale)}>
                {formatShort(last, i18n.locale)}
              </time>
            )}
          </span>
          <span className="flex items-center gap-2">
            <span className="min-w-0 flex-1 truncate text-small text-muted-foreground">
              {c.emails[0] && c.emails[0] !== name ? c.emails[0] : known ? (c.external_ids[0]?.external_id ?? "") : <Trans>Visitor</Trans>}
            </span>
            {c.activity && c.activity.open_conversations > 0 && (
              <span className="shrink-0 text-caption text-brand">
                <Plural value={c.activity.open_conversations} one="# open" other="# open" />
              </span>
            )}
          </span>
        </span>
      </Link>
    </li>
  )
}

const FILTERS: DirectoryFilter[] = ["all", "open", "known", "visitor"]

export function ContactsColumn() {
  const { t } = useLingui()
  const navigate = useNavigate()
  const currentId = useMatch("/contacts/:contactId")?.params.contactId
  const [q, setQ] = useState("")
  const query = useDebounced(q.trim(), 250)
  const [filter, setFilter] = useState<DirectoryFilter>("all")
  const directory = useContactDirectory(query, filter)
  const items = directory.data?.pages.flatMap((p) => p.items) ?? []
  const [cursor, setCursor] = useState(-1)
  const list = useRef<HTMLUListElement>(null)
  useEffect(() => setCursor(-1), [query, filter])

  useHotkeys({
    j: () => setCursor((i) => Math.min(items.length - 1, i + 1)),
    k: () => setCursor((i) => Math.max(0, i - 1)),
    ArrowDown: () => setCursor((i) => Math.min(items.length - 1, i + 1)),
    ArrowUp: () => setCursor((i) => Math.max(0, i - 1)),
    Enter: () => cursor >= 0 && items[cursor] && navigate(`/contacts/${items[cursor].id}`),
  })
  useEffect(() => {
    if (cursor >= 0) list.current?.querySelectorAll("[data-testid=contact-row]")[cursor]?.scrollIntoView({ block: "nearest" })
  }, [cursor])

  const labels: Record<DirectoryFilter, string> = { all: t`All`, open: t`Open`, known: t`Known`, visitor: t`Visitors` }
  const titles: Record<DirectoryFilter, string | undefined> = { all: undefined, open: t`Has an open conversation`, known: undefined, visitor: undefined }

  return (
    <nav aria-label={t`Contacts`} className="flex h-full min-h-0 flex-col bg-background" data-testid="contacts-page">
      <div className="flex h-15 shrink-0 items-center ps-4 pe-2">
        <h2 className="min-w-0 flex-1 truncate text-title">
          <Trans>Contacts</Trans>
        </h2>
      </div>
      <div className="shrink-0 px-3 pb-2">
        <InputGroup className="h-9 rounded-xl border-transparent bg-muted shadow-none focus-within:border-border focus-within:bg-card dark:bg-muted">
          <InputGroupAddon>
            <SearchIcon className="size-[15px] text-faint" />
          </InputGroupAddon>
          <InputGroupInput
            value={q}
            onChange={(e) => setQ(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Escape") {
                if (q) setQ("")
                else e.currentTarget.blur()
              }
              if (e.key === "ArrowDown") {
                e.preventDefault()
                e.currentTarget.blur()
                setCursor(0)
              }
              if (e.key === "Enter" && items[0]) {
                e.preventDefault()
                navigate(`/contacts/${items[cursor >= 0 ? cursor : 0].id}`)
              }
            }}
            placeholder={t`Name, e-mail or id`}
            aria-label={t`Search contacts`}
            data-testid="contacts-search"
          />
          {q && (
            <InputGroupAddon align="inline-end">
              <InputGroupButton size="icon-xs" onClick={() => setQ("")} aria-label={t`Clear the search`}>
                <XIcon />
              </InputGroupButton>
            </InputGroupAddon>
          )}
        </InputGroup>
      </div>
      <div className="flex shrink-0 flex-wrap gap-1.5 px-3 pb-2.5" role="toolbar" aria-label={t`Show`}>
        {FILTERS.map((k) => (
          <button
            key={k}
            type="button"
            aria-pressed={filter === k}
            title={titles[k]}
            onClick={() => setFilter(k)}
            className={cn(
              "h-7 shrink-0 rounded-full px-3 text-small font-medium transition-colors",
              filter === k ? "bg-brand-wash text-brand" : "bg-muted text-muted-foreground hover:text-foreground",
            )}
            data-testid={`contacts-filter-${k}`}
          >
            {labels[k]}
          </button>
        ))}
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto px-2 pb-4">
        {directory.isPending ? (
          <div className="flex flex-col gap-2 p-2">
            <Skeleton className="h-12 w-full" />
            <Skeleton className="h-12 w-full" />
          </div>
        ) : directory.error ? (
          <ErrorLine error={directory.error} className="p-3" />
        ) : items.length === 0 ? (
          <p className="px-4 py-10 text-center text-body text-faint">
            {query ? (
              <Trans>No contact matches “{query}”.</Trans>
            ) : filter !== "all" ? (
              <Trans>No contacts in this view.</Trans>
            ) : (
              <Trans>No contacts yet. They appear when someone writes in.</Trans>
            )}
          </p>
        ) : (
          <ul ref={list} className="flex flex-col">
            {items.map((c, i) => (
              <Row key={c.id} c={c} current={c.id === currentId} cursor={i === cursor} />
            ))}
          </ul>
        )}
        {directory.hasNextPage && (
          <div className="mt-2 flex justify-center">
            <Button variant="ghost" size="sm" onClick={() => void directory.fetchNextPage()} disabled={directory.isFetchingNextPage}>
              <Trans>Load more</Trans>
            </Button>
          </div>
        )}
      </div>
    </nav>
  )
}

export function ContactsHome() {
  return (
    <PaneEmpty icon={BookUserIcon} title={<Trans>Pick a contact</Trans>} testId="contacts-home">
      <Trans>Search by name, e-mail or the id your app gives them, then open one to see their conversations, details and notes.</Trans>
    </PaneEmpty>
  )
}
