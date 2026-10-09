import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { SearchIcon } from "lucide-react"
import { useEffect, useRef, useState } from "react"
import { Link, useNavigate } from "react-router"

import { ContactAvatar, ErrorLine, Segmented } from "@/components/common"
import { formatDateTime, formatShort } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/components/ui/input-group"
import { Skeleton } from "@/components/ui/skeleton"
import { useContactDirectory, type DirectoryFilter } from "@/features/contact/queries"
import { contactName } from "@/features/contact/Section"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useIsPhone } from "@/hooks/use-media-query"
import type { Contact } from "@/lib/api"
import { useInboxes } from "@/lib/workspace"

function useDebounced<T>(value: T, ms: number) {
  const [v, setV] = useState(value)
  useEffect(() => {
    const id = setTimeout(() => setV(value), ms)
    return () => clearTimeout(id)
  }, [value, ms])
  return v
}

const cols = "grid grid-cols-[minmax(0,2fr)_minmax(0,1.1fr)_minmax(0,0.9fr)_minmax(0,0.8fr)] gap-3 phone:grid-cols-[minmax(0,1fr)_auto]"

function Row({ c, active }: { c: Contact; active: boolean }) {
  const { t, i18n } = useLingui()
  const inboxes = useInboxes().data ?? []
  const name = contactName(c, t`Unnamed contact`)
  const known = c.emails.length > 0 || c.external_ids.length > 0
  const apps = [...new Set(c.external_ids.map((x) => inboxes.find((i) => i.id === x.inbox_id)?.name).filter(Boolean))].join(", ")
  return (
    <li>
      <Link
        to={`/contacts/${c.id}`}
        className={`${cols} items-center px-4 py-3 transition-colors hover:bg-muted data-[active=true]:bg-muted`}
        data-active={active}
        data-testid="contact-row"
      >
        <span className="flex min-w-0 items-center gap-2.5">
          <ContactAvatar id={c.id} name={name} className="size-8 shrink-0" />
          <span className="min-w-0">
            <span className="block truncate font-medium">{name}</span>
            <span className="block truncate text-small text-muted-foreground">
              {c.emails[0] && c.emails[0] !== name ? c.emails[0] : known ? (c.external_ids[0]?.external_id ?? "") : <Trans>Visitor</Trans>}
            </span>
          </span>
        </span>
        <span className="truncate text-small text-muted-foreground phone:hidden">{apps || "—"}</span>
        <span className="truncate text-small text-muted-foreground phone:hidden">
          {c.activity?.last_conversation_at ? (
            <span title={formatDateTime(c.activity.last_conversation_at, i18n.locale)}>{formatShort(c.activity.last_conversation_at, i18n.locale)}</span>
          ) : (
            "—"
          )}
        </span>
        <span className="text-caption whitespace-nowrap">
          {c.activity && c.activity.open_conversations > 0 ? (
            <span className="text-brand">
              <Plural value={c.activity.open_conversations} one="# open" other="# open" />
            </span>
          ) : c.activity && c.activity.conversations > 0 ? (
            <span className="text-faint">
              <Trans>Nothing open</Trans>
            </span>
          ) : (
            <span className="text-faint">
              <Trans>No conversations</Trans>
            </span>
          )}
        </span>
      </Link>
    </li>
  )
}

export function ContactsPage() {
  const { t } = useLingui()
  const navigate = useNavigate()
  const [q, setQ] = useState("")
  const query = useDebounced(q.trim(), 250)
  const [filter, setFilter] = useState<DirectoryFilter>("all")
  const phone = useIsPhone()
  const directory = useContactDirectory(query, filter)
  const items = directory.data?.pages.flatMap((p) => p.items) ?? []
  const [cursor, setCursor] = useState(-1)
  const search = useRef<HTMLInputElement>(null)
  useEffect(() => setCursor(-1), [query, filter])

  useHotkeys({
    j: () => setCursor((i) => Math.min(items.length - 1, i + 1)),
    k: () => setCursor((i) => Math.max(0, i - 1)),
    Enter: () => cursor >= 0 && items[cursor] && navigate(`/contacts/${items[cursor].id}`),
  })
  useEffect(() => {
    if (cursor >= 0) document.querySelectorAll("[data-testid=contact-row]")[cursor]?.scrollIntoView({ block: "nearest" })
  }, [cursor])

  return (
    <main className="mx-auto w-full max-w-[1040px] px-6 pt-6 pb-20 phone:px-4 phone:pt-4" data-testid="contacts-page">
      <div className="flex flex-wrap items-center gap-3">
        <h1 className="flex-1 text-page">
          <Trans>Contacts</Trans>
        </h1>
        <InputGroup className="h-9 max-w-[360px] flex-[1_1_260px]">
          <InputGroupAddon>
            <SearchIcon />
          </InputGroupAddon>
          <InputGroupInput
            ref={search}
            value={q}
            onChange={(e) => setQ(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Escape") (e.currentTarget as HTMLInputElement).blur()
              if (e.key === "ArrowDown") {
                e.preventDefault()
                ;(e.currentTarget as HTMLInputElement).blur()
                setCursor(0)
              }
            }}
            placeholder={t`Name, e-mail or id`}
            aria-label={t`Search contacts`}
            data-testid="contacts-search"
          />
        </InputGroup>
      </div>

      <Segmented<DirectoryFilter>
        label={t`Show`}
        value={filter}
        onChange={setFilter}
        className="mt-4 max-w-full"
        items={[
          { value: "all", label: t`All`, testId: "contacts-filter-all" },
          { value: "open", label: phone ? t`Open` : t`Has an open conversation`, testId: "contacts-filter-open" },
          { value: "known", label: t`Known`, testId: "contacts-filter-known" },
          { value: "visitor", label: t`Visitors`, testId: "contacts-filter-visitor" },
        ]}
      />

      <div className="mt-3 overflow-hidden rounded-2xl border">
        <div className={`${cols} border-b bg-surface px-4 py-2.5 text-caption text-faint`}>
          <span>
            <Trans>Contact</Trans>
          </span>
          <span className="phone:hidden">
            <Trans>App ids</Trans>
          </span>
          <span className="phone:hidden">
            <Trans>Last conversation</Trans>
          </span>
          <span>
            <Trans>Status</Trans>
          </span>
        </div>
        {directory.isPending ? (
          <div className="flex flex-col gap-2 p-4">
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-10 w-full" />
          </div>
        ) : directory.error ? (
          <ErrorLine error={directory.error} className="p-4" />
        ) : items.length === 0 ? (
          <p className="px-4 py-10 text-center text-muted-foreground">
            {query ? (
              <Trans>No contact matches “{query}”.</Trans>
            ) : filter !== "all" ? (
              <Trans>No contacts in this view.</Trans>
            ) : (
              <Trans>No contacts yet. They appear when someone writes in.</Trans>
            )}
          </p>
        ) : (
          <ul className="divide-y">
            {items.map((c, i) => (
              <Row key={c.id} c={c} active={i === cursor} />
            ))}
          </ul>
        )}
      </div>
      {directory.hasNextPage && (
        <div className="mt-3 flex justify-center">
          <Button variant="ghost" size="sm" onClick={() => void directory.fetchNextPage()} disabled={directory.isFetchingNextPage}>
            <Trans>Load more</Trans>
          </Button>
        </div>
      )}
    </main>
  )
}
