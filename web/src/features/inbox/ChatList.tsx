import { Trans, useLingui } from "@lingui/react/macro"
import {
  BookUserIcon,
  BotIcon,
  ChevronDownIcon,
  ClockIcon,
  CommandIcon,
  EllipsisIcon,
  InboxIcon,
  KeyboardIcon,
  MegaphoneIcon,
  PinIcon,
  ReplyIcon,
  SearchIcon,
  SettingsIcon,
  Trash2Icon,
  XIcon,
} from "lucide-react"
import { useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react"
import { createPortal } from "react-dom"
import { useNavigate } from "react-router"

import { ListPager } from "@/components/common/ListPager"
import { SearchError, isSearchError } from "@/components/common/SearchError"
import { useShell } from "@/app/shell"
import {
  Dot,
  ErrorLine,
  FlatTime,
  ImportedChip,
  Kbd,
  MemberAvatar,
  placeOf,
  SlaBadge,
  sourceOf,
  toast,
  useContactName,
  useSourceText,
} from "@/components/common"
import {
  boxRowClass,
  BulkBar,
  bulkButton,
  FilterRow,
  filterButton,
  filterButtonOn,
  KeyHints,
  ListBox,
  ListBoxEmpty,
  ListBoxSkeleton,
  RowCheckbox,
} from "@/components/common/ListBox"
import { keyLabel, mod, SHORTCUTS } from "@/components/common/ShortcutSheet"
import {
  formatDateTime,
  formatDuration,
  formatShort,
  useEnumText,
  useErrorText,
  usePreviewText,
} from "@/components/common/text"
import { useLatest } from "@/hooks/use-latest"
import { useNow } from "@/hooks/use-now"
import { useConfirm } from "@/components/common/ConfirmDialog"
import { Alert } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { ButtonGroup } from "@/components/ui/button-group"
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuCheckboxItem,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from "@/components/ui/input-group"
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { NewConversationDialog } from "@/features/contact/ContactDetails"
import { ContactSearch } from "@/features/contact/ContactSearch"
import { useDraftText } from "@/features/conversation/actions"
import { useSnoozeTimes } from "@/features/conversation/HeaderActions"
import { TrashLine } from "@/features/conversation/Trash"
import { TrashStrip, useTrashTools } from "@/features/conversation/TrashBulk"
import { useLiveIds } from "@/features/conversation/liveness"
import { Ticks } from "@/features/conversation/messages/MessageActions"
import { currentRating, RatingMark } from "@/features/conversation/rating"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { useListCursor } from "@/features/inbox/listCursor"
import { LinkIncidentDialog } from "@/features/status/LinkIncidentDialog"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { ImportBanner } from "@/features/migrations/ImportBanner"
import { useActiveView } from "@/features/inbox/views/useActiveView"
import { ViewsMenu } from "@/features/inbox/views/ViewsMenu"
import { ProductTag } from "@/features/inbox/ProductTag"
import { LabelTag, look } from "@/features/inbox/PageFrame"
import { useListActions } from "@/features/inbox/memberState"
import { setListQuery, useListSearch } from "@/features/inbox/listSearch"
import { BulkFailuresDialog } from "@/features/fields/BulkFailures"
import { SourceFilter, sourceFilters, useOpenOnly, useOrigin, useSourceFilter } from "@/features/inbox/SourceFilter"
import { useBulkTeam, useBulkUpdateConversations, useConversations, useCounts } from "@/features/inbox/queries"
import type { ChannelKind, ConversationBulkFailure, Contact, ConversationListItem, ConversationStatus } from "@/lib/api"
import type { ConversationFilters } from "@/lib/keys"
import { useSetAvailability } from "@/lib/availability"
import { useSession } from "@/lib/session"
import { useTyping } from "@/lib/typing"
import { cn } from "@/lib/utils"
import { useChannelMap, useLabels, useMemberMap, useMyTeams, useTeams } from "@/lib/workspace"

export type Chip = "waiting" | "all" | "mine" | "teams" | "unassigned" | "snoozed" | "done" | "spam" | "trash"
export const PAGE_CHIPS: Chip[] = ["waiting", "all", "mine", "teams", "unassigned", "snoozed", "done"]

export function useChips(): Chip[] {
  const { canManage } = useSession()
  const inTeam = useMyTeams().length > 0
  const chips = inTeam ? PAGE_CHIPS : PAGE_CHIPS.filter((c) => c !== "teams")
  return canManage ? [...chips, "spam", "trash"] : [...chips, "spam"]
}

const chipFilters: Record<Chip, ConversationFilters> = {
  waiting: { status: ["open"] },
  all: {},
  mine: { status: ["open"], assignee: "me" },
  teams: { status: ["open"], team: "mine" },
  unassigned: { status: ["open"], assignee: "unassigned" },
  snoozed: { status: ["snoozed"] },
  done: { status: ["closed"] },
  spam: { spam: true },
  trash: { trashed: true, kind: undefined },
}

export function isWaiting(c: ConversationListItem) {
  return c.status === "open" && c.last_message?.author_type === "contact"
}

let remembered: Chip = "waiting"
let rememberedInbox = ""
let focused = 0
const placeListeners = new Set<() => void>()

function subscribePlace(l: () => void) {
  placeListeners.add(l)
  return () => {
    placeListeners.delete(l)
  }
}

export function useRememberedChip(): Chip {
  const offered = useChips()
  const chip = useSyncExternalStore(subscribePlace, () => remembered)
  return offered.includes(chip) ? chip : "waiting"
}

export function useRememberedInbox() {
  return useSyncExternalStore(subscribePlace, () => rememberedInbox)
}

export function rememberChip(c: Chip) {
  remembered = c
  placeListeners.forEach((l) => l())
}

export function rememberInbox(id: string) {
  rememberedInbox = id
  placeListeners.forEach((l) => l())
}

export function useChipLabels(): Record<Chip, string> {
  const { t } = useLingui()
  return {
    waiting: t`Waiting`,
    all: t`All`,
    mine: t`Mine`,
    teams: t`My teams`,
    unassigned: t`Unassigned`,
    snoozed: t`Snoozed`,
    done: t`Closed`,
    spam: t`Spam`,
    trash: t`Trash`,
  }
}

function useDebounced(value: string, ms: number) {
  const [v, setV] = useState(value)
  useEffect(() => {
    const id = setTimeout(() => setV(value), ms)
    return () => clearTimeout(id)
  }, [value, ms])
  return v
}

function Preview({
  c,
  current,
  receipts,
  subject,
}: {
  c: ConversationListItem
  current: boolean
  receipts: boolean
  subject?: string
}) {
  const { t } = useLingui()
  const previewText = usePreviewText()
  const draft = useDraftText(c.id)
  const typing = useTyping(c.id).some((a) => a.type === "contact")
  if (typing) {
    return (
      <span className="truncate text-brand" data-testid="row-typing">
        <Trans>typing…</Trans>
      </span>
    )
  }
  if (draft && !current) {
    return (
      <span className="truncate" data-testid="row-draft">
        <span className="font-medium text-brand">
          <Trans>Draft:</Trans>
        </span>{" "}
        {draft}
      </span>
    )
  }
  if (subject) return <span className="truncate">{subject}</span>
  const m = c.last_message
  if (!m) return <span className="truncate italic">{c.subject || t`No messages yet`}</span>
  return (
    <span className="flex min-w-0 items-center gap-1">
      {m.author_type === "bot" && <BotIcon className="size-3.5 shrink-0" aria-label={t`Bot`} data-testid="row-bot" />}
      {m.author_type === "member" && !receipts && (
        <ReplyIcon className="size-3.5 shrink-0" aria-label={t`Team reply`} />
      )}
      {(m.author_type === "member" || m.author_type === "bot") && receipts && (
        <Ticks at={m.created_at} readAt={c.last_read_by_contact_at} />
      )}
      <span className={cn("truncate", m.redacted && "italic")}>{previewText(m) || c.subject}</span>
    </span>
  )
}

function AwayBanner() {
  const { me } = useSession()
  const set = useSetAvailability()
  if (me.person.availability !== "away") return null
  return (
    <Alert
      role="status"
      className="mx-3 mb-2.5 flex items-center gap-2.5 rounded-[10px] border-0 bg-note py-2.5 ps-3 pe-2 text-small text-note-ink"
      data-testid="away-banner"
    >
      <ClockIcon className="size-3.5 shrink-0" />
      <span className="min-w-0 flex-1">
        <Trans>You are away. Live chat shows nobody available and notifications pause.</Trans>
      </span>
      <Button
        variant="outline"
        size="sm"
        className="h-7 shrink-0 rounded-lg bg-card text-foreground"
        onClick={() => set.mutate("auto")}
        disabled={set.isPending}
      >
        <Trans>I'm back</Trans>
      </Button>
    </Alert>
  )
}

function CleanupBanner({ inboxId }: { inboxId: string }) {
  const { t } = useLingui()
  const errorText = useErrorText()
  const counts = useCounts(["conversation"]).data
  const spam = useConversations(
    { status: ["open"], spam: true, kind: ["conversation"], purpose: "support" },
    !!counts?.spam
  )
  const bulk = useBulkUpdateConversations()
  const items = (spam.data?.pages.flatMap((p) => p.items) ?? []).filter((c) => !inboxId || c.inbox_id === inboxId)
  if (!counts?.spam || items.length === 0) return null
  const ids = items.slice(0, 100).map((c) => c.id)
  const closeAll = () =>
    bulk.mutate(
      { conversation_ids: ids, status: "closed" },
      {
        onSuccess: (r) => {
          const done = r.updated.map((c) => c.id)
          toast(t`${done.length} closed`, () =>
            bulk.mutate({ conversation_ids: done, status: "open" }, { onError: (e) => toast(errorText(e)) })
          )
        },
        onError: (e) => toast(errorText(e)),
      }
    )
  return (
    <Alert
      role="status"
      className="mx-3 mb-2.5 flex flex-wrap items-center gap-2 rounded-[10px] border-dashed border-border bg-card py-2 ps-3 pe-2 text-small"
      data-testid="cleanup-banner"
    >
      <span className="min-w-0 flex-1">
        <b className="font-medium">
          <Trans>Quick cleanup</Trans>
        </b>{" "}
        <span className="text-faint">
          <Trans>{items.length} marked as spam · no reply needed</Trans>
        </span>
      </span>
      <Button
        variant="outline"
        size="sm"
        className={look.btnSm}
        onClick={closeAll}
        disabled={bulk.isPending}
        data-testid="cleanup-close-all"
      >
        <Trans>Close all</Trans>
      </Button>
    </Alert>
  )
}

export function StartConversation({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const [contact, setContact] = useState<Contact | null>(null)
  return (
    <>
      <Dialog
        open={open && !contact}
        onOpenChange={(o) => {
          if (!o) onOpenChange(false)
        }}
      >
        <DialogContent data-testid="start-conversation">
          <DialogHeader>
            <DialogTitle>
              <Trans>Who is it for?</Trans>
            </DialogTitle>
          </DialogHeader>
          {open && <ContactSearch exclude="" onPick={setContact} />}
        </DialogContent>
      </Dialog>
      {contact && (
        <NewConversationDialog
          contact={contact}
          open={open}
          onOpenChange={(o) => {
            if (!o) {
              setContact(null)
              onOpenChange(false)
            }
          }}
        />
      )}
    </>
  )
}

export function ListMenu({ className }: { className?: string }) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const { canManage } = useSession()
  const { openPalette, openShortcuts } = useShell()
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={<Button variant="ghost" size="icon-sm" className={className} />}
        aria-label={t`Menu`}
        title={t`Menu`}
        data-testid="list-menu"
      >
        <EllipsisIcon />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-56">
        <DropdownMenuItem onClick={() => navigate("/contacts")} data-testid="open-contacts">
          <BookUserIcon />
          <Trans>Contacts</Trans>
          <DropdownMenuShortcut>{keyLabel(SHORTCUTS.contacts).join(" ")}</DropdownMenuShortcut>
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => navigate("/settings")}>
          <SettingsIcon />
          <Trans>Settings</Trans>
          <DropdownMenuShortcut>{mod} ,</DropdownMenuShortcut>
        </DropdownMenuItem>
        {canManage && (
          <DropdownMenuItem onClick={() => navigate("/setup/new")}>
            <InboxIcon />
            <Trans>Add an inbox</Trans>
          </DropdownMenuItem>
        )}
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={openPalette}>
          <CommandIcon />
          <Trans>Everything</Trans>
          <DropdownMenuShortcut>{mod} K</DropdownMenuShortcut>
        </DropdownMenuItem>
        <DropdownMenuItem onClick={openShortcuts}>
          <KeyboardIcon />
          <Trans>Keyboard shortcuts</Trans>
          <DropdownMenuShortcut>?</DropdownMenuShortcut>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function statusGroups(prev: ConversationListItem[]) {
  const groups = new Map<string, { status: ConversationStatus; until?: string; ids: string[] }>()
  for (const c of prev) {
    const until =
      c.status === "snoozed" && c.snooze_until && Date.parse(c.snooze_until) > Date.now() ? c.snooze_until : undefined
    const status: ConversationStatus = c.status === "snoozed" && !until ? "open" : c.status
    const key = `${status}|${until ?? ""}`
    const g = groups.get(key) ?? { status, until, ids: [] }
    g.ids.push(c.id)
    groups.set(key, g)
  }
  return groups.values()
}

export type ListShown = { shown: number; unread: number; more: boolean }

type Order = "newest" | "oldest" | "sla_due"

function OrderChip({
  order,
  unreadOnly,
  onOrder,
  onUnread,
}: {
  order: Order
  unreadOnly: boolean
  onOrder: (o: Order) => void
  onUnread: (on: boolean) => void
}) {
  const { t } = useLingui()
  const names: Record<Order, string> = { newest: t`Newest first`, oldest: t`Waiting longest`, sla_due: t`Due first` }
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button variant="outline" size="sm" className={cn(filterButton, "max-w-52", unreadOnly && filterButtonOn)} />
        }
        aria-label={t`Order and view`}
        data-testid="list-order"
      >
        <span className="truncate">{unreadOnly ? t`Unread · ${names[order]}` : names[order]}</span>
        <ChevronDownIcon className="text-muted-foreground" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-52">
        <DropdownMenuRadioGroup value={order} onValueChange={(v) => onOrder(v as Order)}>
          <DropdownMenuRadioItem value="newest" data-testid="list-order-newest">
            {names.newest}
          </DropdownMenuRadioItem>
          <DropdownMenuRadioItem value="oldest" data-testid="list-order-oldest">
            {names.oldest}
          </DropdownMenuRadioItem>
          <DropdownMenuRadioItem value="sla_due" data-testid="list-order-sla-due">
            {names.sla_due}
          </DropdownMenuRadioItem>
        </DropdownMenuRadioGroup>
        <DropdownMenuSeparator />
        <DropdownMenuCheckboxItem checked={unreadOnly} onCheckedChange={onUnread} data-testid="chip-unread">
          <Trans>Unread only</Trans>
        </DropdownMenuCheckboxItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function ConversationRow({
  c,
  current,
  cursor,
  live,
  checked,
  onOpen,
  onToggle,
}: {
  c: ConversationListItem
  current: boolean
  cursor: boolean
  live: boolean
  checked: boolean
  onOpen: () => void
  onToggle: (range: boolean) => void
}) {
  const { t, i18n } = useLingui()
  const channel = useChannelMap().get(c.channel_id ?? "")
  const members = useMemberMap()
  const assignee = c.assignee_id ? members.get(c.assignee_id) : undefined
  const contactName = useContactName()
  const name = contactName(c, channel, c.contact.name, c.contact.email)
  const source = sourceOf(c, channel)
  const sourceText = useSourceText()
  const place = placeOf(c, channel)
  const rating = currentRating(c)
  const labels = useLabels().data ?? []
  const chips = labels.filter((l) => c.labels.includes(l.id))
  const waiting = isWaiting(c)
  const now = useNow(60_000)
  const subject = c.subject.trim()
  const since = c.assign_waiting_since ? formatDateTime(c.assign_waiting_since, i18n.locale) : ""
  const receipts = channel?.kind === "chat" || channel?.kind === "app"
  const where = [source && sourceText[source], place].filter(Boolean).join(" · ")
  return (
    <div
      data-row
      className={boxRowClass({ current, cursor, checked })}
      data-conversation-id={c.id}
      data-checked={checked || undefined}
    >
      <RowCheckbox checked={checked} name={name} onToggle={onToggle} />
      <button
        type="button"
        onClick={onOpen}
        className="grid min-w-0 flex-1 grid-cols-[8px_minmax(0,1fr)_auto] items-center gap-x-3 gap-y-1 py-2.5 pe-3 text-start text-body leading-5 outline-none [grid-template-areas:'dot_who_time'_'._main_main'_'._meta_meta'] focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset @2xl/list:grid-cols-[8px_minmax(0,190px)_minmax(0,1fr)_auto_auto] @2xl/list:[grid-template-areas:'dot_who_main_meta_time']"
        aria-current={current || undefined}
        data-testid="list-row"
        data-unread={c.unread || undefined}
        data-pinned={c.pinned_at ? true : undefined}
      >
        <span
          className={cn("size-2 rounded-full bg-brand [grid-area:dot]", !c.unread && "invisible")}
          aria-label={c.unread ? t`Unread` : undefined}
          data-testid={c.unread ? "row-unread" : undefined}
        />
        <span className="flex min-w-0 flex-col [grid-area:who]">
          <span className="flex min-w-0 items-center gap-1.5">
            {c.pinned_at && (
              <PinIcon
                className="size-3 shrink-0 rotate-45 text-faint"
                aria-label={t`Pinned`}
                data-testid="row-pinned"
              />
            )}
            <span className={cn("min-w-0 truncate", c.unread ? "font-semibold text-foreground" : "font-medium")}>
              {name}
            </span>
            {live && (
              <span className="size-2 shrink-0 rounded-full bg-success" title={t`Online now`} data-testid="row-live" />
            )}
          </span>
          <span
            className="flex min-w-0 items-center gap-1.5 text-caption text-muted-foreground"
            data-testid="row-source"
          >
            <ProductTag id={c.inbox_id} className="h-4 px-1 text-[10.5px]" />
            {c.imported && <ImportedChip at={c.imported.at} />}
            {c.broadcast_id && (
              <MegaphoneIcon
                className="size-3.5 shrink-0 text-brand"
                aria-label={t`Reply to a broadcast`}
                data-testid="row-broadcast"
              />
            )}
            <span className="min-w-0 truncate">{where}</span>
          </span>
        </span>
        <span className="flex min-w-0 flex-col [grid-area:main]">
          {subject && (
            <span className="flex min-w-0 items-center gap-1.5">
              {c.trashed_at && <Trash2Icon className="size-3.5 shrink-0 text-faint" aria-label={t`Trash`} />}
              {c.status === "snoozed" && <ClockIcon className="size-3.5 shrink-0 text-faint" aria-label={t`Snoozed`} />}
              <span className={cn("min-w-0 truncate text-foreground", c.unread ? "font-semibold" : "font-medium")}>
                {subject}
              </span>
            </span>
          )}
          <span className="flex min-w-0 text-muted-foreground">
            <Preview c={c} current={current} receipts={receipts} />
          </span>
          {c.trashed_at && <TrashLine c={c} />}
        </span>
        <span className="flex min-w-0 items-center gap-2 [grid-area:meta] empty:hidden @2xl/list:justify-end">
          {c.priority === "urgent" && (
            <span className="shrink-0 rounded-full bg-destructive/10 px-1.5 text-[11px] leading-[18px] font-semibold text-destructive">
              <Trans>Urgent</Trans>
            </span>
          )}
          {chips.length > 0 && (
            <span className="hidden min-w-0 items-center gap-1 @3xl/list:flex">
              {chips.slice(0, 2).map((l) => (
                <LabelTag key={l.id} name={l.name} />
              ))}
            </span>
          )}
          {c.sla && <SlaBadge sla={c.sla} className="@2xl/list:max-w-48" />}
          {rating && <RatingMark rating={rating.rating} comment={rating.comment} />}
          {!assignee && c.assign_waiting_since && (
            <ClockIcon
              className="size-3.5 shrink-0 text-warning"
              aria-label={t`Waiting for an available teammate since ${since}`}
              data-testid="row-assign-waiting"
            />
          )}
          <span
            className="shrink-0"
            title={assignee ? t`Assigned to ${assignee.name || assignee.email}` : t`Unassigned`}
            data-testid={assignee ? "row-assignee" : undefined}
          >
            {assignee ? (
              <MemberAvatar name={assignee.name || assignee.email} className="size-5 text-[9px]" />
            ) : (
              <span className="grid size-5 place-items-center rounded-full border border-dashed border-input text-[10px] text-faint">
                –
              </span>
            )}
          </span>
        </span>
        <FlatTime
          iso={c.last_activity_at}
          className={cn(
            "justify-self-end [grid-area:time] @2xl/list:w-16 @2xl/list:text-end",
            waiting && "font-medium text-brand"
          )}
        >
          {waiting
            ? formatDuration(Math.max(60, (now - Date.parse(c.last_activity_at)) / 1000), i18n.locale)
            : formatShort(c.last_activity_at, i18n.locale)}
        </FlatTime>
      </button>
    </div>
  )
}

type StatusPick = ConversationStatus | "any"

const STATUS_PICKS: StatusPick[] = ["any", "open", "pending", "snoozed", "closed"]

const CHANNELS: ("all" | ChannelKind)[] = ["all", "email", "chat", "app"]

export function ChatList({
  currentId,
  onOpen,
  active = true,
  chip: controlledChip,
  inbox: controlledInbox,
  onInbox,
  hidden = false,
  head,
  pagerSlot,
  place,
}: {
  currentId: string | null
  onOpen: (id: string) => void
  active?: boolean
  chip?: Chip
  inbox?: string
  onInbox?: (id: string) => void
  hidden?: boolean
  head?: React.ReactNode | ((shown: ListShown) => React.ReactNode)
  pagerSlot?: HTMLElement | null
  place?: string
}) {
  const { t } = useLingui()
  const errorText = useErrorText()
  const enumText = useEnumText()
  const sourceText = useSourceText()
  const { canManage, membership } = useSession()
  const [confirm, confirmDialog] = useConfirm()
  const initialChip = useRememberedChip()
  const { query, focus, select } = useListSearch()
  const q = useDebounced(query.trim(), 250)
  const [sidebarInbox] = useInboxFilter()
  const [ownInbox, setOwnInbox] = useState("")
  const chipInbox = controlledInbox ?? ownInbox
  const inboxId = sidebarInbox || chipInbox
  const chip = controlledChip ?? initialChip
  const [order, setOrder] = useState<Order | null>(null)
  const [unreadOnly, setUnreadOnly] = useState(false)
  const [labelId, setLabelId] = useState("")
  const [teamId, setTeamId] = useState("")
  const [statusPick, setStatusPick] = useState<StatusPick | null>(null)
  const search = useRef<HTMLInputElement>(null)
  const scroller = useRef<HTMLDivElement>(null)
  const sentinel = useRef<HTMLDivElement>(null)
  const [nearEnd, setNearEnd] = useState(false)
  const [selecting, setSelecting] = useState(false)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const anchor = useRef<string | null>(null)
  const [linking, setLinking] = useState(false)
  const view = useActiveView()
  const openOnly = useOpenOnly()
  const sort: Order =
    order ?? (view.id ? (view.sla ? "sla_due" : "newest") : chip === "waiting" && !openOnly ? "oldest" : "newest")

  const [source, setSource] = useSourceFilter()
  const [origin] = useOrigin()
  const narrowed =
    !!origin ||
    !!source.source ||
    !!source.site ||
    !!source.broadcast ||
    !!source.field ||
    !!source.sla ||
    !!labelId ||
    !!teamId ||
    statusPick !== null ||
    !!chipInbox
  const own = { ...sourceFilters(source), origin }
  const picked: Pick<ConversationFilters, "status"> =
    statusPick === null ? {} : { status: statusPick === "any" ? undefined : [statusPick] }
  const filters: ConversationFilters = view.base
    ? {
        kind: ["conversation"],
        ...view.base,
        ...Object.fromEntries(
          Object.entries({
            ...own,
            inbox_id: inboxId || undefined,
            label_id: labelId || undefined,
            team_id: teamId || undefined,
          }).filter(([, v]) => v !== undefined)
        ),
        ...picked,
        fields: view.base.fields || own.fields ? { ...view.base.fields, ...own.fields } : undefined,
        sort: sort === "sla_due" ? "sla_due" : undefined,
        q: q || undefined,
        purpose: q ? view.base.purpose : (view.base.purpose ?? "support"),
      }
    : {
        kind: ["conversation"],
        ...(openOnly ? { status: ["open" as const] } : chipFilters[chip]),
        ...own,
        ...picked,
        inbox_id: inboxId || undefined,
        label_id: labelId || undefined,
        team_id: teamId || undefined,
        sort: sort === "sla_due" ? "sla_due" : undefined,
        q: q || undefined,
        purpose: q ? undefined : "support",
      }
  const list = useConversations(filters)
  const pinnedList = useConversations({ ...filters, pinned: true })
  const loaded = useMemo(() => list.data?.pages.flatMap((p) => p.items) ?? [], [list.data])
  const pinnedLoaded = useMemo(() => pinnedList.data?.pages.flatMap((p) => p.items) ?? [], [pinnedList.data])
  const keep = (c: ConversationListItem) =>
    (!unreadOnly || c.unread) &&
    (view.id !== null || openOnly || statusPick !== null || chip !== "waiting" || isWaiting(c))
  const byTime = (a: ConversationListItem, b: ConversationListItem) =>
    (a.last_activity_at < b.last_activity_at ? 1 : -1) * (sort === "oldest" ? -1 : 1)
  const pinnedRows = [...pinnedLoaded, ...loaded.filter((c) => c.pinned_at && !pinnedLoaded.some((x) => x.id === c.id))]
    .filter((c) => c.pinned_at && keep(c))
    .sort(sort === "sla_due" ? () => 0 : byTime)
  const mainRows = loaded.filter((c) => !c.pinned_at && keep(c))
  if (sort === "oldest") mainRows.sort(byTime)
  const rows = [...pinnedRows, ...mainRows]
  const live = useLiveIds(rows.filter((c) => c.status !== "closed"))
  const { cursorId, setCursorId, moveCursor, openCursor, step } = useListCursor(
    rows,
    currentId,
    onOpen,
    active,
    scroller,
    !hidden,
    !hidden
  )
  const toTop = () => window.scrollTo({ top: 0 })

  const scope = [
    origin,
    chip,
    view.id,
    q,
    inboxId,
    labelId,
    teamId,
    statusPick,
    unreadOnly,
    source.source,
    source.site,
    source.broadcast?.id,
    source.field?.key,
    source.field?.value,
    source.sla,
    sort,
  ].join("|")
  const shown = useRef({ scope, ids: [] as string[] })
  useEffect(() => {
    const before = shown.current
    const ids = rows.map((c) => c.id)
    shown.current = { scope, ids }
    if (!active || !currentId || selecting || list.isPending || before.scope !== scope) return
    const at = before.ids.indexOf(currentId)
    if (at < 0 || ids.includes(currentId)) return
    const next = ids[Math.min(at, ids.length - 1)]
    if (next) onOpen(next)
  })

  const stopSelecting = () => {
    setSelecting(false)
    setSelected(new Set())
    anchor.current = null
  }
  const toggle = (id: string, range = false) => {
    const ids = rows.map((c) => c.id)
    const from = anchor.current ? ids.indexOf(anchor.current) : -1
    const to = ids.indexOf(id)
    const next = new Set(selected)
    if (range && from >= 0 && to >= 0) {
      for (let i = Math.min(from, to); i <= Math.max(from, to); i++) next.add(ids[i])
    } else if (!next.delete(id)) next.add(id)
    anchor.current = id
    setSelected(next)
    setSelecting(next.size > 0)
  }
  const chosenRows = rows.filter((c) => selected.has(c.id))
  const allPicked = rows.length > 0 && chosenRows.length === rows.length
  const chosen = chosenRows.map((c) => c.id)
  const none = chosen.length === 0
  const toggleAll = () => {
    if (allPicked) stopSelecting()
    else {
      setSelected(new Set(rows.map((c) => c.id)))
      setSelecting(true)
    }
  }

  const [selectSeen, setSelectSeen] = useState(select)
  if (select !== selectSeen) {
    setSelectSeen(select)
    setSelecting(true)
  }

  useEffect(() => {
    if (focus <= focused) return
    focused = focus
    requestAnimationFrame(() => search.current?.focus())
  }, [focus])

  useEffect(() => {
    const el = sentinel.current
    if (!el || typeof IntersectionObserver === "undefined") return
    const io = new IntersectionObserver((entries) => setNearEnd(entries.some((e) => e.isIntersecting)), {
      rootMargin: "400px",
    })
    io.observe(el)
    return () => io.disconnect()
  }, [])
  const { hasNextPage, isFetchingNextPage, fetchNextPage } = list
  useEffect(() => {
    if (nearEnd && hasNextPage && !isFetchingNextPage) void fetchNextPage()
  }, [nearEnd, hasNextPage, isFetchingNextPage, fetchNextPage, loaded.length])

  const reset = () => {
    setCursorId(null)
    toTop()
  }
  const seenView = useRef(view.id)
  const changeView = useLatest(() => {
    setSource({})
    if (view.id) setListQuery(view.q)
    setCursorId(null)
  })
  useEffect(() => {
    const was = seenView.current
    seenView.current = view.id
    if (!view.id && !was) return
    setOrder(null)
    setLabelId("")
    setTeamId("")
    setStatusPick(null)
    setUnreadOnly(false)
    changeView.current()
    window.scrollTo({ top: 0 })
  }, [view.id, changeView])
  const setInbox = (id: string) => {
    setOwnInbox(id)
    onInbox?.(id)
    reset()
  }
  const [shownChip, setShownChip] = useState(chip)
  if (shownChip !== chip) {
    setShownChip(chip)
    setStatusPick(null)
    setCursorId(null)
  }
  useEffect(() => {
    window.scrollTo({ top: 0 })
  }, [chip])

  const empty: Record<Chip, React.ReactNode> = {
    waiting: <Trans>Nobody is waiting for a reply.</Trans>,
    all: <Trans>No conversations yet.</Trans>,
    mine: <Trans>Nothing is assigned to you.</Trans>,
    teams: <Trans>Nothing is assigned to your teams.</Trans>,
    unassigned: <Trans>Every open conversation has someone.</Trans>,
    snoozed: <Trans>Nothing is snoozed.</Trans>,
    done: <Trans>Nothing is closed yet.</Trans>,
    spam: <Trans>No spam here.</Trans>,
    trash: <Trans>The Trash is empty.</Trans>,
  }
  const filtered = !!q || narrowed || unreadOnly
  const clearFilters = () => {
    setListQuery("")
    setLabelId("")
    setTeamId("")
    setStatusPick(null)
    setSource({})
    setUnreadOnly(false)
    setInbox("")
    view.clear()
  }

  const bulk = useBulkUpdateConversations()
  const memberActions = useListActions()
  const times = useSnoozeTimes()
  const failed = (e: unknown) => toast(errorText(e))
  const restoreStatus = (prev: ConversationListItem[]) => {
    for (const g of statusGroups(prev))
      bulk.mutate({ conversation_ids: g.ids, status: g.status, snooze_until: g.until }, { onError: failed })
  }
  const [failures, setFailures] = useState<ConversationBulkFailure[]>([])
  const changeStatus = (
    prev: ConversationListItem[],
    change: { status: ConversationStatus; snooze_until?: string },
    done: (n: number) => string
  ) => {
    if (prev.length === 0) return
    bulk.mutate(
      { conversation_ids: prev.map((c) => c.id), ...change },
      {
        onSuccess: (r) => {
          const ids = new Set(r.updated.map((c) => c.id))
          if (ids.size > 0) toast(done(ids.size), () => restoreStatus(prev.filter((c) => ids.has(c.id))))
          if (r.failed.length > 0) setFailures(r.failed)
          stopSelecting()
        },
        onError: failed,
      }
    )
  }
  const closeChosen = () =>
    changeStatus(
      chosenRows.filter((c) => c.status !== "closed").slice(0, 100),
      { status: "closed" },
      (n) => t`${n} closed`
    )
  const snoozeChosen = (at: Date, when: string) =>
    changeStatus(
      chosenRows.slice(0, 100),
      { status: "snoozed", snooze_until: at.toISOString() },
      (n) => t`${n} snoozed, back ${when}`
    )
  const me = membership.member_id
  const allMine = chosenRows.length > 0 && chosenRows.every((c) => c.assignee_id === me)
  const assignChosen = () => {
    const prev = chosenRows.filter((c) => c.assignee_id !== me).slice(0, 100)
    if (prev.length === 0) return
    bulk.mutate(
      { conversation_ids: prev.map((c) => c.id), assignee_id: me },
      {
        onSuccess: (r) => {
          const ids = new Set(r.updated.map((c) => c.id))
          const n = ids.size
          const back = new Map<string | null, string[]>()
          for (const c of prev.filter((x) => ids.has(x.id))) {
            const key = c.assignee_id ?? null
            back.set(key, [...(back.get(key) ?? []), c.id])
          }
          if (n > 0)
            toast(t`${n} assigned to you`, () => {
              for (const [assignee, list] of back)
                bulk.mutate({ conversation_ids: list, assignee_id: assignee }, { onError: failed })
            })
          if (r.failed.length > 0) setFailures(r.failed)
          stopSelecting()
        },
        onError: failed,
      }
    )
  }
  const anyUnread = chosenRows.some((c) => c.unread)
  const readChosen = () => {
    const ids = chosenRows.filter((c) => c.unread === anyUnread).map((c) => c.id)
    const n = ids.length
    if (n === 0) return
    for (const id of ids) {
      if (anyUnread) memberActions.markRead(id)
      else memberActions.markUnread(id)
    }
    const undo = () => {
      for (const id of ids) {
        if (anyUnread) memberActions.markUnread(id)
        else memberActions.markRead(id)
      }
    }
    toast(anyUnread ? t`${n} marked as read` : t`${n} marked as unread`, undo)
    stopSelecting()
  }
  const labelChosen = (labelIdToToggle: string) => {
    const all = chosenRows.every((c) => c.labels.includes(labelIdToToggle))
    const targets = chosenRows.filter((c) => c.labels.includes(labelIdToToggle) === all).map((c) => c.id)
    const body = (add: boolean) => (add ? { add_labels: [labelIdToToggle] } : { remove_labels: [labelIdToToggle] })
    bulk.mutate(
      { conversation_ids: targets.slice(0, 100), ...body(!all) },
      {
        onSuccess: () => {
          toast(t`Labels changed`, () =>
            bulk.mutate({ conversation_ids: targets.slice(0, 100), ...body(all) }, { onError: failed })
          )
          stopSelecting()
        },
        onError: failed,
      }
    )
  }
  const allLabels = useLabels().data ?? []
  const allTeams = useTeams().data ?? []
  const bulkTeam = useBulkTeam()
  const teamsForAll = allTeams.filter((team) => chosenRows.every((c) => team.inbox_ids.includes(c.inbox_id)))
  const teamChosen = (id: string | null) =>
    bulkTeam.mutate(
      { ids: chosen.slice(0, 100), teamId: id },
      {
        onSuccess: () => {
          toast(t`Team changed`)
          stopSelecting()
        },
        onError: failed,
      }
    )
  const inTrash = view.base ? !!view.base.trashed : chip === "trash"
  const inSpam = view.base ? !!view.base.spam && !view.base.trashed : chip === "spam"
  const trashTools = useTrashTools()
  const setTrashed = async (ids: string[], trashed: boolean) => {
    const done: string[] = []
    try {
      for (let at = 0; at < ids.length; at += 100) {
        const r = await bulk.mutateAsync({ conversation_ids: ids.slice(at, at + 100), trashed })
        done.push(...r.updated.map((c) => c.id))
      }
    } catch (e) {
      failed(e)
    }
    return done
  }
  const trashChosen = () => {
    const ids = [...chosen]
    if (ids.length === 0) return
    confirm({
      title: <Trans>Move {ids.length} to the Trash?</Trans>,
      description: (
        <Trans>
          They disappear for your team and for the customers. You can restore them from the Trash for 30 days, then they
          are deleted for good with their messages and files.
        </Trans>
      ),
      confirm: <Trans>Move to the Trash</Trans>,
      run: () =>
        void setTrashed(ids, true).then((done) => {
          if (done.length === 0) return
          toast(t`${done.length} moved to the Trash`, () => void setTrashed(done, false))
          stopSelecting()
        }),
    })
  }
  const restoreChosen = () => {
    const ids = [...chosen]
    if (ids.length === 0) return
    void setTrashed(ids, false).then((done) => {
      if (done.length === 0) return
      toast(t`${done.length} restored`, () => void setTrashed(done, true))
      stopSelecting()
    })
  }
  const setSpam = (ids: string[], spam: boolean) =>
    bulk.mutate(
      { conversation_ids: ids.slice(0, 100), spam },
      {
        onSuccess: (r) => {
          const done = r.updated.map((c) => c.id)
          if (done.length === 0) return
          toast(spam ? t`${done.length} marked as spam` : t`${done.length} marked as not spam`, () =>
            bulk.mutate({ conversation_ids: done, spam: !spam }, { onError: failed })
          )
          stopSelecting()
        },
        onError: failed,
      }
    )
  const notSpamChosen = () => chosen.length > 0 && setSpam([...chosen], false)
  const deleteChosen = () =>
    trashTools.deleteForever([...chosen], (id) => chosenRows.find((c) => c.id === id)?.subject || id, stopSelecting)
  useHotkeys(
    {
      [SHORTCUTS.select]: () => {
        const id = cursorId ?? currentId
        if (id) toggle(id)
        else setSelecting(true)
      },
      ...(selecting ? { Escape: stopSelecting } : {}),
      ...(selecting && !none
        ? {
            [SHORTCUTS.linkSelected]: () => setLinking(true),
            ...(!inTrash && !inSpam
              ? { [SHORTCUTS.close]: closeChosen, [SHORTCUTS.unread]: readChosen, [SHORTCUTS.hand]: assignChosen }
              : {}),
            ...(canManage && !inSpam
              ? { [inTrash ? SHORTCUTS.restore : SHORTCUTS.trash]: inTrash ? restoreChosen : trashChosen }
              : {}),
            ...(canManage && inSpam ? { [SHORTCUTS.trash]: trashChosen } : {}),
            ...(inSpam ? { [SHORTCUTS.spam]: notSpamChosen } : {}),
            ...(canManage && inTrash ? { [SHORTCUTS.deleteForever]: deleteChosen } : {}),
          }
        : {}),
    },
    active && !hidden
  )

  const searchBox = (
    <InputGroup className="h-8 min-w-48 flex-[1_1_240px] rounded-md border-input bg-card shadow-none focus-within:border-faint">
      <InputGroupAddon>
        <SearchIcon className="size-[15px] text-faint" />
      </InputGroupAddon>
      <InputGroupInput
        ref={search}
        name="list-search"
        value={query}
        onChange={(e) => {
          setListQuery(e.target.value)
          setCursorId(null)
        }}
        onKeyDown={(e) => {
          if (e.key === "ArrowDown" || e.key === "ArrowUp") {
            e.preventDefault()
            moveCursor(e.key === "ArrowDown" ? 1 : -1)
          } else if (e.key === "Enter") {
            e.preventDefault()
            openCursor()
          } else if (e.key === "Escape") {
            if (query) setListQuery("")
            else e.currentTarget.blur()
          }
        }}
        placeholder={t`Search people, subjects, messages`}
        aria-label={t`Search conversations`}
        data-testid="list-search"
      />
      <InputGroupAddon align="inline-end">
        {query ? (
          <InputGroupButton size="icon-xs" onClick={() => setListQuery("")} aria-label={t`Clear the search`}>
            <XIcon />
          </InputGroupButton>
        ) : (
          <Kbd className="phone:hidden">/</Kbd>
        )}
      </InputGroupAddon>
    </InputGroup>
  )

  const viewStatus = (view.base?.status ?? (openOnly ? ["open"] : chipFilters[chip].status)) as
    | ConversationStatus[]
    | undefined
  const statusValue: StatusPick = statusPick ?? (viewStatus?.length === 1 ? viewStatus[0] : "any")
  const statusNames: Record<StatusPick, string> = { any: t`Any status`, ...enumText.status }
  const channel = source.source ?? "all"
  const filterRow = (
    <FilterRow testId="chips">
      {searchBox}
      <ToggleGroup
        value={[channel]}
        onValueChange={(v) => {
          const next = v[0] as (typeof CHANNELS)[number] | undefined
          if (!next) return
          setSource({ ...source, source: next === "all" ? undefined : next })
          reset()
        }}
        aria-label={t`Channel`}
        size="sm"
        spacing={0}
        variant="outline"
        className="bg-card"
        data-testid="channel-filter"
      >
        {CHANNELS.map((k) => (
          <ToggleGroupItem
            key={k}
            value={k}
            className="h-8 px-2.5 font-normal aria-pressed:bg-muted aria-pressed:font-medium aria-pressed:text-foreground"
            data-testid={`channel-${k}`}
          >
            {k === "all" ? <Trans>All</Trans> : sourceText[k]}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
      {!inTrash && !inSpam && (
        <NativeSelect
          size="sm"
          value={statusValue}
          onChange={(e) => {
            setStatusPick(e.target.value as StatusPick)
            reset()
          }}
          aria-label={t`Status`}
          className={cn("bg-card", statusPick !== null && "[&>select]:border-brand/40 [&>select]:text-brand")}
          data-testid="status-filter"
        >
          {STATUS_PICKS.map((s) => (
            <NativeSelectOption key={s} value={s}>
              {statusNames[s]}
            </NativeSelectOption>
          ))}
        </NativeSelect>
      )}
      {allLabels.length > 0 && (
        <NativeSelect
          size="sm"
          value={labelId}
          onChange={(e) => {
            setLabelId(e.target.value)
            reset()
          }}
          aria-label={t`Filter by label`}
          className={cn("bg-card", labelId && "[&>select]:border-brand/40 [&>select]:text-brand")}
          data-testid="label-filter"
        >
          <NativeSelectOption value="">{t`Any label`}</NativeSelectOption>
          {allLabels.map((l) => (
            <NativeSelectOption key={l.id} value={l.id}>
              {l.name}
            </NativeSelectOption>
          ))}
        </NativeSelect>
      )}
      {allTeams.length > 0 && (
        <NativeSelect
          size="sm"
          value={teamId}
          onChange={(e) => {
            setTeamId(e.target.value)
            reset()
          }}
          aria-label={t`Filter by team`}
          className={cn("bg-card", teamId && "[&>select]:border-brand/40 [&>select]:text-brand")}
          data-testid="team-filter"
        >
          <NativeSelectOption value="">{t`Any team`}</NativeSelectOption>
          {allTeams.map((team) => (
            <NativeSelectOption key={team.id} value={team.id}>
              {team.name}
            </NativeSelectOption>
          ))}
        </NativeSelect>
      )}
      <SourceFilter inboxId={inboxId} onChange={reset} types={false} />
      <span className="flex items-center gap-2 @2xl/list:ms-auto">
        <OrderChip
          order={sort}
          unreadOnly={unreadOnly}
          onOrder={(o) => {
            setOrder(o)
            reset()
          }}
          onUnread={(on) => {
            setUnreadOnly(on)
            reset()
          }}
        />
        <ViewsMenu filters={filters} active={active} />
      </span>
      <SearchError error={list.error} className="w-full" />
    </FilterRow>
  )

  const bb = bulkButton()
  const busy = bulk.isPending || bulkTeam.isPending
  const bulkActions = inTrash ? (
    <ButtonGroup>
      <Button
        variant="outline"
        className={bb}
        disabled={none || busy}
        onClick={restoreChosen}
        data-testid="restore-selected"
      >
        <Trans>Restore</Trans>
      </Button>
      {canManage && (
        <Button
          variant="outline"
          className={bulkButton(true)}
          disabled={none || trashTools.pending}
          onClick={deleteChosen}
          title={t`Delete for good (${SHORTCUTS.deleteForever})`}
          data-testid="delete-forever-selected"
        >
          <Trans>Delete forever</Trans>
        </Button>
      )}
    </ButtonGroup>
  ) : inSpam ? (
    <ButtonGroup>
      <Button
        variant="outline"
        className={bb}
        disabled={none || busy}
        onClick={notSpamChosen}
        title={t`Not spam (${SHORTCUTS.spam})`}
        data-testid="not-spam-selected"
      >
        <Trans>Not spam</Trans>
      </Button>
      {canManage && (
        <Button
          variant="outline"
          className={bulkButton(true)}
          disabled={none || busy}
          onClick={trashChosen}
          title={t`Move to the Trash (${SHORTCUTS.trash})`}
          data-testid="trash-selected"
        >
          <Trans>Move to the Trash</Trans>
        </Button>
      )}
    </ButtonGroup>
  ) : (
    <>
      <ButtonGroup>
        <Button
          variant="outline"
          className={bb}
          disabled={none || busy || allMine}
          onClick={assignChosen}
          title={t`Assign to me (${SHORTCUTS.hand})`}
          data-testid="assign-selected"
        >
          <Trans>Assign to me</Trans>
        </Button>
        <DropdownMenu>
          <DropdownMenuTrigger
            render={<Button variant="outline" className={bb} disabled={none || busy} />}
            data-testid="snooze-selected"
          >
            <Trans>Snooze</Trans>
            <ChevronDownIcon className="text-muted-foreground" />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className="min-w-56">
            <DropdownMenuGroup>
              <DropdownMenuLabel>
                <Trans>When should they come back?</Trans>
              </DropdownMenuLabel>
              {times.map((o) => (
                <DropdownMenuItem key={o.label} onClick={() => snoozeChosen(o.at, o.when)}>
                  <span className="flex-1">{o.label}</span>
                  {o.detail && <span className="text-caption text-faint">{o.detail}</span>}
                </DropdownMenuItem>
              ))}
            </DropdownMenuGroup>
          </DropdownMenuContent>
        </DropdownMenu>
        <Button
          variant="outline"
          className={bb}
          disabled={none || busy}
          onClick={closeChosen}
          title={t`Close (${SHORTCUTS.close})`}
          data-testid="close-selected"
        >
          <Trans>Close</Trans>
        </Button>
      </ButtonGroup>
      <ButtonGroup>
        <DropdownMenu>
          <DropdownMenuTrigger
            render={<Button variant="outline" className={bb} disabled={none || busy} />}
            data-testid="label-selected"
          >
            <Trans>Label</Trans>
            <ChevronDownIcon className="text-muted-foreground" />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className="max-h-[60svh] min-w-48 overflow-y-auto">
            {allLabels.length === 0 && (
              <p className="px-2.5 py-1.5 text-body text-muted-foreground">
                <Trans>No labels yet. Add them in settings.</Trans>
              </p>
            )}
            {allLabels.map((l) => (
              <DropdownMenuCheckboxItem
                key={l.id}
                checked={chosenRows.length > 0 && chosenRows.every((c) => c.labels.includes(l.id))}
                onCheckedChange={() => labelChosen(l.id)}
              >
                <Dot color={l.color} />
                {l.name}
              </DropdownMenuCheckboxItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
        <Button
          variant="outline"
          className={bb}
          disabled={none}
          onClick={readChosen}
          title={t`Mark as read or unread (${SHORTCUTS.unread})`}
          data-testid="read-selected"
        >
          {anyUnread || none ? <Trans>Mark as read</Trans> : <Trans>Mark as unread</Trans>}
        </Button>
        <DropdownMenu>
          <DropdownMenuTrigger
            render={<Button variant="outline" size="icon-sm" className="bg-card" disabled={none} />}
            aria-label={t`More`}
            title={t`More`}
            data-testid="more-selected"
          >
            <EllipsisIcon />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="min-w-52">
            {allTeams.length > 0 && (
              <DropdownMenuSub>
                <DropdownMenuSubTrigger data-testid="team-selected" disabled={bulkTeam.isPending}>
                  <span className="flex-1">
                    <Trans>Team</Trans>
                  </span>
                </DropdownMenuSubTrigger>
                <DropdownMenuSubContent className="max-h-[60svh] min-w-48 overflow-y-auto">
                  {teamsForAll.length === 0 && (
                    <p className="px-2.5 py-1.5 text-body text-muted-foreground">
                      <Trans>No team covers all the selected inboxes.</Trans>
                    </p>
                  )}
                  {teamsForAll.map((team) => (
                    <DropdownMenuItem key={team.id} onClick={() => teamChosen(team.id)}>
                      {team.name}
                    </DropdownMenuItem>
                  ))}
                  {teamsForAll.length > 0 && (
                    <>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem onClick={() => teamChosen(null)}>
                        <Trans>Remove the team</Trans>
                      </DropdownMenuItem>
                    </>
                  )}
                </DropdownMenuSubContent>
              </DropdownMenuSub>
            )}
            <DropdownMenuItem onClick={() => setLinking(true)} data-testid="link-selected">
              <Trans>Link to an incident</Trans>
              <DropdownMenuShortcut>{keyLabel(SHORTCUTS.linkSelected).join("")}</DropdownMenuShortcut>
            </DropdownMenuItem>
            {canManage && (
              <>
                <DropdownMenuSeparator />
                <DropdownMenuItem
                  variant="destructive"
                  disabled={bulk.isPending}
                  onClick={trashChosen}
                  data-testid="trash-selected"
                >
                  <Trans>Delete</Trans>
                  <DropdownMenuShortcut>{SHORTCUTS.trash}</DropdownMenuShortcut>
                </DropdownMenuItem>
              </>
            )}
          </DropdownMenuContent>
        </DropdownMenu>
      </ButtonGroup>
    </>
  )

  const row = (c: ConversationListItem) => (
    <ConversationRow
      key={c.id}
      c={c}
      current={c.id === currentId}
      cursor={c.id === cursorId}
      live={live.has(c.id)}
      checked={selected.has(c.id)}
      onOpen={() => onOpen(c.id)}
      onToggle={(range) => toggle(c.id, range)}
    />
  )

  const at = currentId ? rows.findIndex((c) => c.id === currentId) : -1
  const pager =
    pagerSlot &&
    currentId &&
    createPortal(
      <ListPager
        at={at}
        total={rows.length}
        more={!!hasNextPage}
        place={place ?? t`Conversations`}
        onPrevious={() => step(-1)}
        onNext={() => step(1)}
        previousLabel={t`Previous conversation (${SHORTCUTS.previous})`}
        nextLabel={t`Next conversation (${SHORTCUTS.next})`}
      />,
      pagerSlot
    )

  return (
    <nav
      aria-label={t`Conversations`}
      className={cn("@container/list flex min-w-0 flex-col", hidden && "hidden")}
      data-testid="chat-list"
    >
      {pager}
      {typeof head === "function"
        ? head({ shown: rows.length, unread: rows.filter((c) => c.unread).length, more: !!hasNextPage })
        : head}
      {filterRow}
      <div className="mt-3 flex flex-col gap-2 empty:hidden [&>*]:mx-0 [&>*]:mb-0">
        <AwayBanner />
        {!view.id && !openOnly && chip === "waiting" && <CleanupBanner inboxId={inboxId} />}
        {inTrash && canManage && <TrashStrip onEmpty={(n) => trashTools.emptyTrash(n)} busy={trashTools.pending} />}
        <ImportBanner />
      </div>
      {trashTools.dialogs}
      <div ref={scroller} data-testid="list">
        <ListBox
          bar={
            selecting && (
              <BulkBar
                picked={chosen.length}
                total={rows.length}
                onToggleAll={toggleAll}
                onCancel={stopSelecting}
                busy={busy}
              >
                {bulkActions}
              </BulkBar>
            )
          }
          testId="list-box"
        >
          {list.isPending ? (
            <ListBoxSkeleton />
          ) : rows.length === 0 && !hasNextPage ? (
            <ListBoxEmpty
              testId="list-empty"
              action={
                filtered && (
                  <Button variant="outline" size="sm" onClick={clearFilters} data-testid="clear-filters">
                    <Trans>Clear search and filters</Trans>
                  </Button>
                )
              }
            >
              {q ? (
                <Trans>Nothing matches your search here.</Trans>
              ) : narrowed || unreadOnly ? (
                <Trans>Nothing matches these filters.</Trans>
              ) : (
                empty[view.id ? "all" : chip]
              )}
            </ListBoxEmpty>
          ) : (
            rows.map(row)
          )}
          {isFetchingNextPage && <ListBoxSkeleton rows={1} />}
        </ListBox>
        <div ref={sentinel} className="h-px" />
        <ErrorLine error={isSearchError(list.error) ? null : list.error} className="py-2" />
      </div>
      <KeyHints
        items={[
          [[SHORTCUTS.next, SHORTCUTS.previous], <Trans key="move">move</Trans>],
          [
            ["↵"],
            <Trans key="open" context="keyboard hint">
              open
            </Trans>,
          ],
          [[SHORTCUTS.select], <Trans key="select">select</Trans>],
          [[SHORTCUTS.close], <Trans key="close">close the selected</Trans>],
          [[SHORTCUTS.search], <Trans key="search">search</Trans>],
        ]}
      />
      {confirmDialog}
      <BulkFailuresDialog failures={failures} rows={rows} onClose={() => setFailures([])} />
      {linking && (
        <LinkIncidentDialog conversationIds={chosen} onClose={() => setLinking(false)} onDone={stopSelecting} />
      )}
    </nav>
  )
}
