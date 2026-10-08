import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { useQuery } from "@tanstack/react-query"
import { ArrowLeftIcon, MailIcon, MergeIcon, SearchIcon } from "lucide-react"
import { useEffect, useState } from "react"

import { ErrorLine, PersonAvatar } from "@/components/common"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { api, unwrap, type Contact } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useContactSearch, useInboxes, useMergeContact } from "@/lib/queries"
import { useSession } from "@/lib/session"

function contactName(c: Contact, fallback: string) {
  return c.name || c.emails[0] || c.external_ids[0]?.external_id || fallback
}

function useConversationCount(contactId: string) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.contactConversationCount(ws, contactId),
    queryFn: () =>
      unwrap(api.GET("/v1/conversations", { params: { query: { contact_id: contactId, limit: 100 } } })).then((r) => ({
        count: r.items.length,
        more: !!r.next_cursor,
      })),
  })
}

function Moves({ title, children }: { title: React.ReactNode; children: React.ReactNode }) {
  return (
    <section className="flex flex-col gap-1">
      <h3 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">{title}</h3>
      {children}
    </section>
  )
}

function Preview({ source, target }: { source: Contact; target: Contact }) {
  const { t } = useLingui()
  const inboxes = useInboxes().data ?? []
  const conversations = useConversationCount(source.id)
  const emails = source.emails.filter((e) => !target.emails.includes(e))
  const own = target.attributes ?? {}
  const attrs = Object.entries(source.attributes ?? {})
  const none = (
    <p className="text-sm text-muted-foreground">
      <Trans>None</Trans>
    </p>
  )
  const count = conversations.data?.count ?? 0
  return (
    <div className="flex min-w-0 flex-col gap-3 rounded-md border p-3" data-testid="merge-preview">
      <Moves title={<Trans>Conversations</Trans>}>
        {conversations.isPending ? (
          <Skeleton className="h-5 w-32" />
        ) : (
          <p className="text-sm" data-testid="merge-conversations">
            {conversations.data?.more ? (
              <Trans>More than {count} conversations</Trans>
            ) : (
              <Plural value={count} _0="No conversations" one="# conversation" other="# conversations" />
            )}
          </p>
        )}
      </Moves>
      <Moves title={<Trans>E-mail addresses</Trans>}>
        {emails.length === 0
          ? none
          : (
            <ul className="flex flex-col gap-0.5">
              {emails.map((e) => (
                <li key={e} className="flex items-center gap-2 text-sm">
                  <MailIcon className="size-3.5 shrink-0 text-muted-foreground" />
                  <span className="truncate">{e}</span>
                </li>
              ))}
            </ul>
          )}
      </Moves>
      <Moves title={<Trans>External ids</Trans>}>
        {source.external_ids.length === 0
          ? none
          : (
            <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 text-sm">
              {source.external_ids.map((x) => (
                <div key={`${x.inbox_id}:${x.external_id}`} className="contents">
                  <dt className="truncate text-muted-foreground">
                    {inboxes.find((i) => i.id === x.inbox_id)?.name ?? "?"}
                  </dt>
                  <dd className="truncate font-mono text-xs leading-5">{x.external_id}</dd>
                </div>
              ))}
            </dl>
          )}
      </Moves>
      <Moves title={<Trans>Attributes</Trans>}>
        {attrs.length === 0
          ? none
          : (
            <dl className="grid grid-cols-[minmax(0,auto)_minmax(0,1fr)] gap-x-3 gap-y-0.5 text-sm">
              {attrs.map(([k, v]) => {
                const kept = k in own
                return (
                  <div key={k} className="contents">
                    <dt className="truncate text-muted-foreground">{k}</dt>
                    <dd className="break-words">
                      {kept ? (
                        <span className="text-muted-foreground line-through" title={t`This contact keeps its own value`}>
                          {typeof v === "object" ? JSON.stringify(v) : String(v)}
                        </span>
                      ) : typeof v === "object" ? (
                        JSON.stringify(v)
                      ) : (
                        String(v)
                      )}
                    </dd>
                  </div>
                )
              })}
            </dl>
          )}
        {attrs.some(([k]) => k in own) && (
          <p className="text-xs text-muted-foreground">
            <Trans>Struck-out values are not taken: this contact keeps its own value for those keys.</Trans>
          </p>
        )}
      </Moves>
    </div>
  )
}

export function MergeContactDialog({
  target,
  open,
  onOpenChange,
}: {
  target: Contact
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useLingui()
  const [q, setQ] = useState("")
  const [query, setQuery] = useState("")
  const [source, setSource] = useState<Contact | null>(null)
  const merge = useMergeContact(target.id)
  const results = useContactSearch(query, open && !source)
  const candidates = (results.data ?? []).filter((c) => c.id !== target.id)
  const unnamed = t`Unnamed contact`
  const targetName = contactName(target, unnamed)
  const sourceName = source ? contactName(source, unnamed) : ""

  useEffect(() => {
    const id = setTimeout(() => setQuery(q.trim()), 250)
    return () => clearTimeout(id)
  }, [q])

  const close = (next: boolean) => {
    if (!next) {
      setQ("")
      setQuery("")
      setSource(null)
      merge.reset()
    }
    onOpenChange(next)
  }

  return (
    <Dialog open={open} onOpenChange={close}>
      <DialogContent className="max-h-[90svh] grid-cols-[minmax(0,1fr)] overflow-y-auto sm:max-w-lg" data-testid="merge-dialog">
        <DialogHeader>
          <DialogTitle className="[overflow-wrap:anywhere]">
            <Trans>Merge a contact into {targetName}</Trans>
          </DialogTitle>
          <DialogDescription className="[overflow-wrap:anywhere]">
            {source ? (
              <Trans>
                Everything below moves from {sourceName} to {targetName}, and {sourceName} is deleted. This cannot be
                undone.
              </Trans>
            ) : (
              <Trans>Pick the other contact. It is merged into this one and then deleted.</Trans>
            )}
          </DialogDescription>
        </DialogHeader>
        {source ? (
          <>
            <div className="flex min-w-0 items-center gap-2 text-sm">
              <span className="flex max-w-[calc(50%-1rem)] min-w-0 items-center gap-2">
                <PersonAvatar name={sourceName} className="size-6 text-[10px]" />
                <span className="min-w-0 truncate font-medium">{sourceName}</span>
              </span>
              <MergeIcon className="size-4 shrink-0 rotate-90 text-muted-foreground" />
              <span className="flex max-w-[calc(50%-1rem)] min-w-0 items-center gap-2">
                <PersonAvatar name={targetName} className="size-6 text-[10px]" />
                <span className="min-w-0 truncate font-medium">{targetName}</span>
              </span>
            </div>
            <Preview source={source} target={target} />
            <ErrorLine error={merge.error} />
            <DialogFooter>
              <Button variant="outline" onClick={() => setSource(null)} disabled={merge.isPending}>
                <ArrowLeftIcon />
                <Trans>Back</Trans>
              </Button>
              <Button
                variant="destructive"
                onClick={() => merge.mutate(source.id, { onSuccess: () => close(false) })}
                disabled={merge.isPending}
                data-testid="merge-confirm"
              >
                <MergeIcon />
                <Trans>Merge</Trans>
              </Button>
            </DialogFooter>
          </>
        ) : (
          <div className="flex flex-col gap-2">
            <div className="relative">
              <SearchIcon className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                type="search"
                value={q}
                onChange={(e) => setQ(e.target.value)}
                placeholder={t`Search by name, e-mail or external id`}
                aria-label={t`Search contacts`}
                className="pl-8"
                autoFocus
                data-testid="merge-search"
              />
            </div>
            <ErrorLine error={results.error} />
            <ul className="-mx-2 flex max-h-72 flex-col overflow-y-auto" data-testid="merge-results">
              {results.isPending && open ? (
                <li className="px-2">
                  <Skeleton className="h-10 w-full" />
                </li>
              ) : candidates.length === 0 ? (
                <li className="px-2 py-3 text-sm text-muted-foreground">
                  <Trans>No other contact matches.</Trans>
                </li>
              ) : (
                candidates.map((c) => {
                  const name = contactName(c, unnamed)
                  const detail = c.emails[0] ?? c.external_ids[0]?.external_id
                  return (
                    <li key={c.id}>
                      <button
                        type="button"
                        className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-muted focus-visible:bg-muted focus-visible:outline-none"
                        onClick={() => setSource(c)}
                        data-testid="merge-candidate"
                      >
                        <PersonAvatar name={name} className="size-7 text-[10px]" />
                        <span className="flex min-w-0 flex-col">
                          <span className="truncate font-medium">{name}</span>
                          {detail && detail !== name && (
                            <span className="truncate text-xs text-muted-foreground">{detail}</span>
                          )}
                        </span>
                      </button>
                    </li>
                  )
                })
              )}
            </ul>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
