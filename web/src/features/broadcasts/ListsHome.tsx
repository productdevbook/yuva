import { plural } from "@lingui/core/macro"
import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation } from "@tanstack/react-query"
import { PlusIcon } from "lucide-react"
import { useMemo, useRef, useState } from "react"
import { Link, useNavigate } from "react-router"

import { ErrorLine, toast } from "@/components/common"
import { FormCard, FormGrid } from "@/components/common/FormCard"
import {
  FilterMenu,
  FilterRow,
  FilterSearch,
  KeyHints,
  ListBox,
  ListBoxEmpty,
  ListBoxSkeleton,
  PageTitle,
} from "@/components/common/ListBox"
import { LIST_KEYS, SHORTCUTS } from "@/components/common/ShortcutSheet"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { keyHint, useNumber } from "@/features/broadcasts/BroadcastParts"
import { BroadcastsLayout, ProductFilter, useProductScope, useViewNames } from "@/features/broadcasts/BroadcastsFrame"
import {
  canSend,
  InboxField,
  NameFields,
  nameTaken,
  RulesToggles,
  SenderPicker,
  type Rules,
} from "@/features/broadcasts/ListForm"
import { ConsentExplainer, NeedsSender } from "@/features/broadcasts/ListParts"
import {
  listLink,
  useArchiveList,
  useGrowth,
  useInboxChannels,
  useLists,
  useSender,
  useSetList,
} from "@/features/broadcasts/queries"
import { useListCursor } from "@/features/inbox/listCursor"
import { idempotencyKey } from "@/features/setup/draft"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { api, unwrap, type MailingList } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

const VIEWS = ["lists", "archived"] as const
type View = (typeof VIEWS)[number]

const head = "h-9 px-3 text-start text-caption font-medium text-muted-foreground"
const cell = "px-3 py-2.5 text-body"
const hideGrowth = "@max-2xl/list:hidden"
const hideLeft = "@max-xl/list:hidden"

function Row({ l, cursor, onOpen }: { l: MailingList; cursor: boolean; onOpen: () => void }) {
  const { t } = useLingui()
  const num = useNumber()
  const { canManage } = useSession()
  const inbox = (useInboxes().data ?? []).find((i) => i.id === l.inbox_id)
  const from = useSender(l.inbox_id, l.channel_id)
  const growth = useGrowth(l.id, 30).data
  const restore = useArchiveList(l.id)
  const fresh = (growth?.days ?? []).reduce((n, d) => n + (l.double_opt_in ? d.confirmed : d.new), 0)
  const added = num(fresh)
  const archived = !!l.archived_at
  const sub = [
    l.double_opt_in ? t`Double opt-in` : t`Single opt-in`,
    l.public ? t`Public` : t`Private`,
    from ? t`From ${from}` : t`No sender yet`,
  ].join(" · ")
  return (
    <TableRow
      onClick={onOpen}
      className={cn(
        "cursor-pointer border-line-soft hover:bg-subtle",
        cursor && "bg-subtle [&>td:first-child]:shadow-[inset_2px_0_0_var(--brand)]"
      )}
      data-row-id={l.id}
      data-testid="lists-card"
    >
      <TableCell className={cn(cell, "min-w-0 overflow-hidden ps-4")}>
        <Link
          to={listLink(l.id)}
          onClick={(e) => e.stopPropagation()}
          title={inbox?.name}
          className="flex min-w-0 flex-col rounded-sm leading-5 outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <span className="flex min-w-0 items-center gap-2">
            <span className="truncate font-medium text-foreground" dir="auto">
              {l.name}
            </span>
            {from && !l.sender_ready && !archived && <NeedsSender />}
          </span>
          <span className={cn("truncate text-small", from ? "text-muted-foreground" : "text-warning")}>{sub}</span>
        </Link>
        <ErrorLine error={restore.error} />
      </TableCell>
      <TableCell className={cn(cell, hideGrowth, "w-32 text-end text-small text-success tabular-nums")}>
        {growth ? <Trans>+{added} this month</Trans> : ""}
      </TableCell>
      <TableCell className={cn(cell, "w-28 text-end text-fg2 tabular-nums")}>{num(l.counts.subscribed)}</TableCell>
      <TableCell className={cn(cell, hideLeft, "w-28 pe-4 text-end text-fg2 tabular-nums")}>
        {archived && canManage ? (
          <Button
            variant="outline"
            size="sm"
            disabled={restore.isPending}
            onClick={(e) => {
              e.stopPropagation()
              restore.mutate(false, { onSuccess: () => toast(t`List restored`) })
            }}
            data-testid="list-restore"
          >
            <Trans>Restore</Trans>
          </Button>
        ) : (
          num(l.counts.unsubscribed)
        )}
      </TableCell>
    </TableRow>
  )
}

function NewListForm({ inboxId, onClose }: { inboxId: string; onClose: () => void }) {
  const navigate = useNavigate()
  const setList = useSetList()
  const inboxes = useInboxes().data ?? []
  const [inbox, setInbox] = useState(inboxId || inboxes[0]?.id || "")
  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [rules, setRules] = useState<Rules>({ double_opt_in: true, public: true, welcome: true, commercial: true })
  const channels = useInboxChannels(inbox).data
  const ready = channels?.find(canSend)
  const [channel, setChannel] = useState<string | null>(null)
  const channelId = channel ?? ready?.id ?? ""
  const [key] = useState(() => crypto.randomUUID())
  const create = useMutation({
    mutationFn: () => {
      const body = {
        inbox_id: inbox,
        name: name.trim(),
        description: description.trim(),
        ...rules,
        channel_id: channelId || undefined,
      }
      return unwrap(
        api.POST("/v1/lists", { body, params: { header: { "Idempotency-Key": idempotencyKey(key, "list", body) } } })
      )
    },
    onSuccess: (l) => {
      setList(l)
      onClose()
      navigate(listLink(l.id))
    },
  })
  return (
    <FormCard
      title={<Trans>New list</Trans>}
      description={
        <Trans>People join it on your forms, by import or through the API. Each keeps a consent record.</Trans>
      }
      submit={<Trans>Create list</Trans>}
      ready={!!name.trim() && !!inbox}
      busy={create.isPending}
      error={nameTaken(create.error) ? null : create.error}
      onCancel={onClose}
      onSubmit={() => create.mutate()}
      testId="new-list"
    >
      <NameFields
        name={name}
        description={description}
        onChange={(p) => {
          if (p.name !== undefined) setName(p.name)
          if (p.description !== undefined) setDescription(p.description)
        }}
        taken={nameTaken(create.error)}
        autoFocus
      />
      <FormGrid>
        {inboxes.length > 1 && (
          <InboxField
            value={inbox}
            onChange={(id) => {
              setInbox(id)
              setChannel(null)
            }}
          />
        )}
        <SenderPicker inboxId={inbox} value={channelId} onChange={setChannel} invalid={false} />
      </FormGrid>
      <RulesToggles values={rules} onChange={(p) => setRules({ ...rules, ...p })} />
    </FormCard>
  )
}

export function ListsOverview() {
  const { t, i18n } = useLingui()
  const navigate = useNavigate()
  const { canManage } = useSession()
  const views = useViewNames()
  const num = useNumber()
  const { chosen, inboxId, params, set } = useProductScope()
  const raw = params.get("tab") as View | null
  const view: View = raw && VIEWS.includes(raw) ? raw : "lists"
  const archived = view === "archived"
  const q = params.get("q") ?? ""
  const lists = useLists(inboxId, archived)
  const open = useLists(inboxId, false)
  const loaded = useMemo(() => lists.data?.pages.flatMap((p) => p.items) ?? [], [lists.data])
  const needle = q.trim().toLowerCase()
  const items = loaded.filter((l) => !needle || l.name.toLowerCase().includes(needle))
  const importable = open.data?.pages.flatMap((p) => p.items) ?? []
  const [creating, setCreating] = useState(false)
  const listRef = useRef<HTMLDivElement>(null)
  const search = useRef<HTMLInputElement>(null)
  const importButton = useRef<HTMLButtonElement>(null)
  const go = (id: string) => navigate(listLink(id))
  const { cursorId, setCursorId } = useListCursor(items, null, go, !creating, listRef, true, true)
  const [scope, setScope] = useState(`${view}|${inboxId}|${q}`)
  if (scope !== `${view}|${inboxId}|${q}`) {
    setScope(`${view}|${inboxId}|${q}`)
    setCursorId(null)
  }
  const startImport = () => {
    if (importable.length === 1) navigate(listLink(importable[0].id, "import"))
    else importButton.current?.click()
  }
  useHotkeys(
    {
      ...(canManage ? { [LIST_KEYS.create]: () => setCreating(true) } : {}),
      ...(canManage && importable.length > 0 ? { [LIST_KEYS.import]: startImport } : {}),
      [LIST_KEYS.lists]: () => set("tab", ""),
      [LIST_KEYS.archived]: () => set("tab", "archived"),
      [SHORTCUTS.search]: () => search.current?.focus(),
    },
    !creating
  )
  const subscribers = num(importable.reduce((n, l) => n + l.counts.subscribed, 0))
  const anyFilter = !!(q || chosen || archived)
  const clearFilters = () => {
    set("q", "")
    set("inbox", "")
    set("tab", "")
  }
  const fmt = new Intl.NumberFormat(i18n.locale)
  const shown = `${fmt.format(items.length)}${lists.hasNextPage ? "+" : ""}`
  return (
    <BroadcastsLayout testId="lists-home">
      <section aria-label={views.lists} className="@container/list min-w-0">
        <PageTitle
          title={views.lists}
          sub={
            <span data-testid="list-lede">
              {open.data ? (
                <Trans>{subscribers} subscribed, each with a consent record</Trans>
              ) : (
                <Trans>Who agreed to hear from you, with consent recorded</Trans>
              )}
            </span>
          }
          actions={
            canManage && (
              <>
                {importable.length > 1 ? (
                  <DropdownMenu>
                    <DropdownMenuTrigger
                      ref={importButton}
                      render={<Button variant="outline" />}
                      title={keyHint(t`Import CSV`, LIST_KEYS.import)}
                      data-testid="lists-home-import"
                    >
                      <Trans>Import CSV</Trans>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end" className="min-w-56" data-testid="lists-import-dialog">
                      <DropdownMenuGroup>
                        <DropdownMenuLabel>
                          <Trans>Pick the list the people join.</Trans>
                        </DropdownMenuLabel>
                        {importable.map((l) => (
                          <DropdownMenuItem key={l.id} onClick={() => navigate(listLink(l.id, "import"))}>
                            <span className="min-w-0 flex-1 truncate">{l.name}</span>
                            <span className="text-small text-muted-foreground tabular-nums">
                              {num(l.counts.subscribed)}
                            </span>
                          </DropdownMenuItem>
                        ))}
                      </DropdownMenuGroup>
                    </DropdownMenuContent>
                  </DropdownMenu>
                ) : (
                  importable.length === 1 && (
                    <Button
                      variant="outline"
                      onClick={startImport}
                      title={keyHint(t`Import CSV`, LIST_KEYS.import)}
                      data-testid="lists-home-import"
                    >
                      <Trans>Import CSV</Trans>
                    </Button>
                  )
                )}
                <Button
                  onClick={() => setCreating(true)}
                  disabled={creating}
                  title={keyHint(t`New list`, LIST_KEYS.create)}
                  data-testid="lists-home-new"
                >
                  <PlusIcon />
                  <Trans>New list</Trans>
                </Button>
              </>
            )
          }
          testId="lists-head"
        />
        {creating && <NewListForm inboxId={inboxId} onClose={() => setCreating(false)} />}
        <FilterRow testId="lists-filters">
          <FilterSearch
            inputRef={search}
            value={q}
            onChange={(v) => set("q", v)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && items[0]) {
                e.preventDefault()
                go(items[0].id)
              }
            }}
            placeholder={t`Search lists…`}
            label={t`Search lists`}
            testId="lists-search"
          />
          <ProductFilter value={chosen} onChange={(id) => set("inbox", id)} testId="lists-product" />
          <FilterMenu
            value={archived ? "archived" : ""}
            onChange={(v) => set("tab", v)}
            options={[{ value: "archived", label: t`Archived` }]}
            anyLabel={t`In use`}
            label={t`Filter by status`}
            testId="lists-state"
          />
          {anyFilter && (
            <Button variant="ghost" size="sm" className="h-8" onClick={clearFilters} data-testid="lists-clear">
              <Trans>Clear filters</Trans>
            </Button>
          )}
        </FilterRow>
        <div ref={listRef}>
          <ListBox testId="lists-table">
            {lists.isPending ? (
              <ListBoxSkeleton rows={3} />
            ) : lists.error && !lists.data ? (
              <ErrorLine error={lists.error} className="px-4 py-4" />
            ) : items.length === 0 ? (
              <ListBoxEmpty
                testId="lists-empty"
                action={
                  anyFilter ? (
                    <Button variant="outline" size="sm" onClick={clearFilters}>
                      <Trans>Clear filters</Trans>
                    </Button>
                  ) : (
                    canManage && (
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => setCreating(true)}
                        data-testid="lists-empty-new"
                      >
                        <Trans>New list</Trans>
                      </Button>
                    )
                  )
                }
              >
                <span className="flex max-w-xl flex-col gap-2">
                  <span className="font-semibold text-foreground">
                    {needle ? (
                      <Trans>No list matches “{q}”.</Trans>
                    ) : archived ? (
                      <Trans>No archived lists</Trans>
                    ) : (
                      <Trans>No mailing lists yet</Trans>
                    )}
                  </span>
                  {!needle &&
                    (archived ? (
                      <Trans>Archived lists keep their subscribers and consent records but take no new sign-ups.</Trans>
                    ) : (
                      <ConsentExplainer />
                    ))}
                </span>
              </ListBoxEmpty>
            ) : (
              <Table className="table-fixed text-body">
                <TableHeader>
                  <TableRow className="border-line-soft hover:bg-transparent">
                    <TableHead className={cn(head, "ps-4")}>
                      <Trans>List</Trans>
                    </TableHead>
                    <TableHead className={cn(head, hideGrowth, "w-32 text-end")}>
                      <Trans>Last 30 days</Trans>
                    </TableHead>
                    <TableHead className={cn(head, "w-28 text-end")}>
                      <Trans>Subscribers</Trans>
                    </TableHead>
                    <TableHead className={cn(head, hideLeft, "w-28 pe-4 text-end")}>
                      {archived && canManage ? "" : <Trans>Unsubscribed</Trans>}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.map((l) => (
                    <Row key={l.id} l={l} cursor={l.id === cursorId} onOpen={() => go(l.id)} />
                  ))}
                </TableBody>
              </Table>
            )}
          </ListBox>
        </div>
        {lists.hasNextPage && (
          <Button
            variant="outline"
            size="sm"
            className="mt-3"
            onClick={() => void lists.fetchNextPage()}
            disabled={lists.isFetchingNextPage}
          >
            <Trans>Load more</Trans>
          </Button>
        )}
        <p className="m-0 mt-3 text-small text-muted-foreground">
          {plural(items.length, { one: `${shown} list`, other: `${shown} lists` })}
          {" · "}
          {canManage ? (
            <Trans>Every subscriber keeps a consent record you can export.</Trans>
          ) : (
            <span data-testid="lists-read-only">
              <Trans>Owners and admins create lists, add subscribers and send broadcasts.</Trans>
            </span>
          )}
        </p>
        <KeyHints
          items={[
            [[SHORTCUTS.next, SHORTCUTS.previous], <Trans key="move">move</Trans>],
            [
              ["↵"],
              <Trans key="open" context="keyboard hint">
                open
              </Trans>,
            ],
            [[SHORTCUTS.search], <Trans key="search">search</Trans>],
          ]}
        />
      </section>
    </BroadcastsLayout>
  )
}
