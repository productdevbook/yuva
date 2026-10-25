import { Trans, useLingui } from "@lingui/react/macro"
import {
  BookUserIcon,
  BotIcon,
  CheckIcon,
  ChevronDownIcon,
  ClockIcon,
  MegaphoneIcon,
  PinIcon,
  CommandIcon,
  EllipsisVerticalIcon,
  InboxIcon,
  KeyboardIcon,
  ListChecksIcon,
  MessageSquarePlusIcon,
  ReplyIcon,
  SearchIcon,
  SettingsIcon,
  SquareCheckBigIcon,
  Trash2Icon,
  XIcon,
} from "lucide-react"
import { useEffect, useMemo, useRef, useState } from "react"
import { Link, useNavigate } from "react-router"

import { SearchError, isSearchError } from "@/components/common/SearchError"
import { useShell } from "@/app/shell"
import { TeamMenu } from "@/app/TeamMenu"
import { UserMenu } from "@/app/UserMenu"
import {
  ContactAvatar,
  Dot,
  ErrorLine,
  ImportedChip,
  isVisitor,
  Kbd,
  MemberAvatar,
  placeOf,
  SlaBadge,
  sourceOf,
  toast,
  useContactName,
  useSourceText,
  VisitorAvatar,
} from "@/components/common"
import { keyLabel, mod, SHORTCUTS } from "@/components/common/ShortcutSheet"
import { formatDateTime, formatDuration, formatShort, useErrorText, usePreviewText } from "@/components/common/text"
import { useLatest } from "@/hooks/use-latest"
import { useNow } from "@/hooks/use-now"
import { useConfirm } from "@/components/common/ConfirmDialog"
import { Alert } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuCheckboxItem,
  DropdownMenuItem,
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
import { Skeleton } from "@/components/ui/skeleton"
import { NewConversationDialog } from "@/features/contact/ContactDetails"
import { ContactSearch } from "@/features/contact/ContactSearch"
import { useDraftText } from "@/features/conversation/actions"
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
import { InboxChip } from "@/features/inbox/InboxPicker"
import { ViewChips } from "@/features/inbox/ListHead"
import { useActiveView } from "@/features/inbox/views/useActiveView"
import { ViewsMenu } from "@/features/inbox/views/ViewsMenu"
import { ProductTag } from "@/features/inbox/ProductTag"
import { LabelTag, look } from "@/features/inbox/PageFrame"
import { useListActions } from "@/features/inbox/memberState"
import { setListQuery, useListSearch } from "@/features/inbox/listSearch"
import { BulkFailuresDialog } from "@/features/fields/BulkFailures"
import { SourceFilter, sourceFilters, useSourceFilter } from "@/features/inbox/SourceFilter"
import {
  useBulkTeam,
  useBulkUpdateConversations,
  useConversations,
  useCounts,
  useLoadedCount,
} from "@/features/inbox/queries"
import type { ConversationBulkFailure, Contact, ConversationListItem, ConversationStatus } from "@/lib/api"
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
let focused = 0

export function useRememberedChip(): Chip {
  const offered = useChips()
  return offered.includes(remembered) ? remembered : "waiting"
}

export function rememberChip(c: Chip) {
  remembered = c
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

function Row({
  c,
  current,
  cursor,
  live,
  selecting,
  checked,
  onOpen,
}: {
  c: ConversationListItem
  current: boolean
  cursor: boolean
  live: boolean
  selecting: boolean
  checked: boolean
  onOpen: (range: boolean) => void
}) {
  const { t, i18n } = useLingui()
  const channel = useChannelMap().get(c.channel_id ?? "")
  const members = useMemberMap()
  const assignee = c.assignee_id ? members.get(c.assignee_id) : undefined
  const contactName = useContactName()
  const visitor = isVisitor(c.contact.name, c.contact.email)
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
  const urgent = c.priority === "urgent"
  return (
    <button
      type="button"
      onClick={(e) => onOpen(e.shiftKey)}
      className={cn(
        "group relative mx-2 flex w-[calc(100%-1rem)] items-start gap-3 rounded-xl px-2.5 py-2.5 text-start outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring",
        current ? "bg-card shadow-[0_1px_2px_rgb(0_0_0/0.05),0_0_0_1px_var(--border)]" : "hover:bg-muted/60",
        cursor && !current && "bg-muted/60",
        (current || cursor) &&
          "before:absolute before:inset-y-3 before:-start-2 before:w-[3px] before:rounded-e-[3px] before:bg-brand",
        checked && "bg-brand/7 shadow-[0_0_0_1px_color-mix(in_srgb,var(--brand)_30%,var(--border))]"
      )}
      aria-current={current || undefined}
      aria-pressed={selecting ? checked : undefined}
      data-checked={checked || undefined}
      data-testid="list-row"
      data-conversation-id={c.id}
      data-unread={c.unread || undefined}
      data-pinned={c.pinned_at ? true : undefined}
    >
      {selecting && (
        <span
          aria-hidden
          className={cn(
            "mt-2.5 grid size-[18px] shrink-0 place-items-center rounded-[5px] border-[1.5px]",
            checked ? "border-foreground bg-foreground text-background" : "border-input bg-card"
          )}
          data-testid="row-check"
        >
          {checked && <CheckIcon className="size-3.5" strokeWidth={3} />}
        </span>
      )}
      <span className="relative shrink-0">
        {visitor ? (
          <VisitorAvatar className="size-[38px]" />
        ) : (
          <ContactAvatar id={c.contact.id} name={name} className="size-[38px] font-semibold" />
        )}
        {live && (
          <span
            className="absolute -end-0.5 bottom-0 size-3 rounded-full border-2 border-background bg-success"
            title={t`Online now`}
            data-testid="row-live"
          />
        )}
      </span>
      <span className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="flex min-w-0 items-center gap-1.5">
          {c.pinned_at && (
            <PinIcon className="size-3 shrink-0 rotate-45 text-faint" aria-label={t`Pinned`} data-testid="row-pinned" />
          )}
          <span
            className={cn(
              "min-w-0 truncate text-body",
              c.unread ? "font-semibold text-foreground" : "font-medium text-foreground/80"
            )}
          >
            {name}
          </span>
          {c.unread && (
            <span className="size-2 shrink-0 rounded-full bg-brand" aria-label={t`Unread`} data-testid="row-unread" />
          )}
          {urgent && (
            <span className="shrink-0 rounded-full bg-destructive/10 px-1.5 text-[11px] leading-[18px] font-semibold text-destructive">
              <Trans>Urgent</Trans>
            </span>
          )}
        </span>
        <span className="flex min-w-0 items-center gap-1.5 text-body text-muted-foreground">
          {c.trashed_at && <Trash2Icon className="size-3.5 shrink-0 text-faint" aria-label={t`Trash`} />}
          {c.status === "snoozed" && <ClockIcon className="size-3.5 shrink-0 text-faint" aria-label={t`Snoozed`} />}
          <span className="flex min-w-0 flex-1">
            <Preview
              c={c}
              current={current}
              subject={subject}
              receipts={channel?.kind === "chat" || channel?.kind === "app"}
            />
          </span>
        </span>
        {c.trashed_at && <TrashLine c={c} />}
        <span className="mt-0.5 flex min-w-0 items-center gap-1.5 text-caption" data-testid="row-source">
          <ProductTag id={c.inbox_id} />
          {c.imported && <ImportedChip at={c.imported.at} />}
          {source && <span className="shrink-0 text-faint">{sourceText[source]}</span>}
          {c.broadcast_id && (
            <MegaphoneIcon
              className="size-3.5 shrink-0 text-brand"
              aria-label={t`Reply to a broadcast`}
              data-testid="row-broadcast"
            />
          )}
          {place && <span className="min-w-0 truncate text-faint">{place}</span>}
          {chips.map((l) => (
            <LabelTag key={l.id} name={l.name} />
          ))}
          <span className="flex-1" />
          {c.sla && <SlaBadge sla={c.sla} />}
          {rating && <RatingMark rating={rating.rating} comment={rating.comment} />}
          {!assignee && c.assign_waiting_since && (
            <span
              className="inline-flex shrink-0 items-center gap-1 text-warning"
              title={t`Waiting for an available teammate since ${since}`}
              data-testid="row-assign-waiting"
            >
              <ClockIcon className="size-3" />
              <Trans>Waiting for a teammate</Trans>
            </span>
          )}
        </span>
      </span>
      <span className="flex shrink-0 flex-col items-end gap-1.5 pt-px">
        <time
          className={cn(
            "font-mono text-[12px] whitespace-nowrap tabular-nums",
            waiting ? "font-medium text-brand" : "text-faint"
          )}
          dateTime={c.last_activity_at}
        >
          {waiting
            ? formatDuration(Math.max(60, (now - Date.parse(c.last_activity_at)) / 1000), i18n.locale)
            : formatShort(c.last_activity_at, i18n.locale)}
        </time>
        {assignee && (
          <span title={t`Assigned to ${assignee.name || assignee.email}`} data-testid="row-assignee">
            <MemberAvatar name={assignee.name || assignee.email} className="size-5 text-[9px]" />
          </span>
        )}
      </span>
    </button>
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
        <EllipsisVerticalIcon />
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

function PhoneHeader({ children }: { children: React.ReactNode }) {
  const { t } = useLingui()
  const { membership } = useSession()
  const [starting, setStarting] = useState(false)
  return (
    <div className="flex shrink-0 flex-col border-b">
      <div className="flex h-15 shrink-0 items-center gap-1 ps-2 pe-1.5">
        <UserMenu align="start" />
        <span className="min-w-0 flex-1 truncate ps-1 text-title" data-testid="workspace-name">
          {membership.workspace.name}
        </span>
        <TeamMenu />
        <Button
          variant="ghost"
          size="icon-sm"
          onClick={() => setStarting(true)}
          aria-label={t`New conversation`}
          title={t`New conversation`}
          data-testid="new-conversation"
        >
          <MessageSquarePlusIcon />
        </Button>
        <Button
          variant="ghost"
          size="icon-sm"
          render={<Link to="/settings" />}
          aria-label={t`Settings`}
          title={t`Settings (${mod},)`}
          data-testid="open-settings"
        >
          <SettingsIcon />
        </Button>
        <ListMenu />
      </div>
      <div className="px-3 [&>[role=toolbar]]:mt-0">{children}</div>
      <StartConversation open={starting} onOpenChange={setStarting} />
    </div>
  )
}

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
        render={<button type="button" className={cn(unreadOnly ? look.listChipOn : look.listChip, "ms-auto")} />}
        aria-label={t`Order and view`}
        data-testid="list-order"
      >
        <span className="truncate">{unreadOnly ? t`Unread · ${names[order]}` : names[order]}</span>
        <ChevronDownIcon className="size-3.5 shrink-0 text-faint" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="min-w-52">
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

function LabelChip({ value, onChange }: { value: string; onChange: (id: string) => void }) {
  const { t } = useLingui()
  const labels = useLabels().data ?? []
  const current = labels.find((l) => l.id === value)
  if (labels.length === 0) return null
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={<button type="button" className={current ? look.listChipOn : look.listChip} />}
        aria-label={t`Filter by label`}
        title={t`Filter by label`}
        data-testid="label-filter"
      >
        {current ? (
          <>
            <Dot color={current.color} className="size-2" />
            <span className="max-w-24 truncate">{current.name}</span>
          </>
        ) : (
          <Trans>Label</Trans>
        )}
        <ChevronDownIcon className="size-3.5 shrink-0 text-faint" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="max-h-[60svh] min-w-48">
        <DropdownMenuRadioGroup value={value} onValueChange={(v) => onChange(String(v))}>
          <DropdownMenuRadioItem value="">
            <Trans>Any label</Trans>
          </DropdownMenuRadioItem>
          {labels.map((l) => (
            <DropdownMenuRadioItem key={l.id} value={l.id}>
              <Dot color={l.color} />
              <span className="truncate">{l.name}</span>
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function TeamChip({ value, onChange }: { value: string; onChange: (id: string) => void }) {
  const { t } = useLingui()
  const teams = useTeams().data ?? []
  const current = teams.find((x) => x.id === value)
  if (teams.length === 0) return null
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={<button type="button" className={current ? look.listChipOn : look.listChip} />}
        aria-label={t`Filter by team`}
        title={t`Filter by team`}
        data-testid="team-filter"
      >
        {current ? <span className="max-w-24 truncate">{current.name}</span> : <Trans>Team</Trans>}
        <ChevronDownIcon className="size-3.5 shrink-0 text-faint" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="max-h-[60svh] min-w-48">
        <DropdownMenuRadioGroup value={value} onValueChange={(v) => onChange(String(v))}>
          <DropdownMenuRadioItem value="">
            <Trans>Any team</Trans>
          </DropdownMenuRadioItem>
          {teams.map((team) => (
            <DropdownMenuRadioItem key={team.id} value={team.id}>
              <span className="truncate">{team.name}</span>
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
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

export function ChatList({
  currentId,
  onOpen,
  active = true,
  chip: controlledChip,
  onChip,
  inbox: controlledInbox,
  onInbox,
  embedded = false,
  head,
}: {
  currentId: string | null
  onOpen: (id: string) => void
  active?: boolean
  chip?: Chip
  onChip?: (c: Chip) => void
  inbox?: string
  onInbox?: (id: string) => void
  embedded?: boolean
  head?: React.ReactNode
}) {
  const { t } = useLingui()
  const errorText = useErrorText()
  const { canManage } = useSession()
  const [confirm, confirmDialog] = useConfirm()
  const tabs = useChips()
  const initialChip = useRememberedChip()
  const { query, focus, select } = useListSearch()
  const q = useDebounced(query.trim(), 250)
  const [sidebarInbox] = useInboxFilter()
  const [ownInbox, setOwnInbox] = useState("")
  const chipInbox = controlledInbox ?? ownInbox
  const locked = !!sidebarInbox
  const inboxId = sidebarInbox || chipInbox
  const [ownChip, setOwnChip] = useState<Chip>(initialChip)
  const chip = controlledChip ?? ownChip
  const [order, setOrder] = useState<Order | null>(null)
  const [unreadOnly, setUnreadOnly] = useState(false)
  const [labelId, setLabelId] = useState("")
  const [teamId, setTeamId] = useState("")
  const search = useRef<HTMLInputElement>(null)
  const scroller = useRef<HTMLDivElement>(null)
  const sentinel = useRef<HTMLDivElement>(null)
  const [nearEnd, setNearEnd] = useState(false)
  const [selecting, setSelecting] = useState(false)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const anchor = useRef<string | null>(null)
  const [linking, setLinking] = useState(false)
  const view = useActiveView()
  const sort: Order = order ?? (view.id ? (view.sla ? "sla_due" : "newest") : chip === "waiting" ? "oldest" : "newest")

  const [source, setSource] = useSourceFilter()
  const narrowed =
    !!source.source ||
    !!source.site ||
    !!source.broadcast ||
    !!source.field ||
    !!source.sla ||
    !!labelId ||
    !!teamId ||
    !!chipInbox
  const own = sourceFilters(source)
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
        fields: view.base.fields || own.fields ? { ...view.base.fields, ...own.fields } : undefined,
        sort: sort === "sla_due" ? "sla_due" : undefined,
        q: q || undefined,
        purpose: q ? view.base.purpose : (view.base.purpose ?? "support"),
      }
    : {
        kind: ["conversation"],
        ...chipFilters[chip],
        ...own,
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
    (!unreadOnly || c.unread) && (view.id !== null || chip !== "waiting" || isWaiting(c))
  const byTime = (a: ConversationListItem, b: ConversationListItem) =>
    (a.last_activity_at < b.last_activity_at ? 1 : -1) * (sort === "oldest" ? -1 : 1)
  const pinnedRows = [...pinnedLoaded, ...loaded.filter((c) => c.pinned_at && !pinnedLoaded.some((x) => x.id === c.id))]
    .filter((c) => c.pinned_at && keep(c))
    .sort(sort === "sla_due" ? () => 0 : byTime)
  const mainRows = loaded.filter((c) => !c.pinned_at && keep(c))
  if (sort === "oldest") mainRows.sort(byTime)
  const rows = [...pinnedRows, ...mainRows]
  const live = useLiveIds(rows.filter((c) => c.status !== "closed"))
  const { cursorId, setCursorId, moveCursor, openCursor } = useListCursor(rows, currentId, onOpen, active, scroller)

  const scope = [
    chip,
    view.id,
    q,
    inboxId,
    labelId,
    teamId,
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

  const toggle = (id: string, range = false) => {
    setSelecting(true)
    const ids = rows.map((c) => c.id)
    const from = anchor.current ? ids.indexOf(anchor.current) : -1
    const to = ids.indexOf(id)
    setSelected((s) => {
      const next = new Set(s)
      if (range && from >= 0 && to >= 0) {
        for (let i = Math.min(from, to); i <= Math.max(from, to); i++) next.add(ids[i])
      } else if (!next.delete(id)) next.add(id)
      return next
    })
    anchor.current = id
  }
  const stopSelecting = () => {
    setSelecting(false)
    setSelected(new Set())
    anchor.current = null
  }
  const linkKey = keyLabel(SHORTCUTS.linkSelected).join("")
  const chosenRows = rows.filter((c) => selected.has(c.id))
  const allPicked = rows.length > 0 && chosenRows.length === rows.length
  const chosen = chosenRows.map((c) => c.id)
  const picked = chosen.length
  useHotkeys(
    {
      [SHORTCUTS.select]: () => {
        const id = cursorId ?? currentId
        if (id) toggle(id)
        else setSelecting(true)
      },
      ...(selecting
        ? {
            Escape: stopSelecting,
            ...(chosen.length > 0 ? { [SHORTCUTS.linkSelected]: () => setLinking(true) } : {}),
          }
        : {}),
    },
    active
  )

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
    const root = scroller.current
    if (!el || !root || typeof IntersectionObserver === "undefined") return
    const io = new IntersectionObserver((entries) => setNearEnd(entries.some((e) => e.isIntersecting)), {
      root,
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
    scroller.current?.scrollTo({ top: 0 })
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
    setUnreadOnly(false)
    changeView.current()
    scroller.current?.scrollTo({ top: 0 })
  }, [view.id, changeView])
  const setChip = (c: Chip) => {
    view.clear()
    rememberChip(c)
    setOwnChip(c)
    onChip?.(c)
    reset()
  }
  const setInbox = (id: string) => {
    setOwnInbox(id)
    onInbox?.(id)
    reset()
  }
  const [shownChip, setShownChip] = useState(chip)
  if (shownChip !== chip) {
    setShownChip(chip)
    setCursorId(null)
  }
  useEffect(() => {
    scroller.current?.scrollTo({ top: 0 })
  }, [chip])

  const labels = useChipLabels()
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
    setSource({})
    setUnreadOnly(false)
    setInbox("")
    view.clear()
  }

  const bulk = useBulkUpdateConversations()
  const memberActions = useListActions()
  const failed = (e: unknown) => toast(errorText(e))
  const restoreStatus = (prev: ConversationListItem[]) => {
    for (const g of statusGroups(prev))
      bulk.mutate({ conversation_ids: g.ids, status: g.status, snooze_until: g.until }, { onError: failed })
  }
  const [failures, setFailures] = useState<ConversationBulkFailure[]>([])
  const closeChosen = () => {
    const prev = chosenRows.filter((c) => c.status !== "closed").slice(0, 100)
    if (prev.length === 0) return
    bulk.mutate(
      { conversation_ids: prev.map((c) => c.id), status: "closed" },
      {
        onSuccess: (r) => {
          const done = new Set(r.updated.map((c) => c.id))
          if (done.size > 0) toast(t`${done.size} closed`, () => restoreStatus(prev.filter((c) => done.has(c.id))))
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
  const teamChosen = (teamId: string | null) =>
    bulkTeam.mutate(
      { ids: chosen.slice(0, 100), teamId },
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
    selecting && chosen.length > 0
      ? {
          ...(canManage && !inSpam
            ? { [inTrash ? SHORTCUTS.restore : SHORTCUTS.trash]: inTrash ? restoreChosen : trashChosen }
            : {}),
          ...(canManage && inSpam ? { [SHORTCUTS.trash]: trashChosen } : {}),
          ...(inSpam ? { [SHORTCUTS.spam]: notSpamChosen } : {}),
          ...(canManage && inTrash ? { [SHORTCUTS.deleteForever]: deleteChosen } : {}),
        }
      : {},
    active
  )

  const searchBox = (
    <InputGroup className="h-8 min-w-0 flex-1 rounded-[9px] border-input bg-card shadow-none focus-within:border-faint">
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
        placeholder={t`Search`}
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

  const searchRow = (
    <div className="flex shrink-0 flex-wrap items-center gap-1.5 px-3 pt-3 pb-2">
      {searchBox}
      <Button
        variant="outline"
        className={cn("h-8 w-8 rounded-[9px] border-input bg-card px-0 font-normal", selecting && "bg-muted")}
        onClick={() => (selecting ? stopSelecting() : setSelecting(true))}
        aria-pressed={selecting}
        aria-label={t`Select conversations`}
        title={t`Select conversations`}
        data-testid="select-conversations"
      >
        <SquareCheckBigIcon />
      </Button>
      <SearchError error={list.error} className="w-full" />
    </div>
  )

  const none = picked === 0
  const bb =
    "h-[34px] rounded-lg px-2.5 font-normal text-background hover:bg-background/15 hover:text-background aria-expanded:bg-background/15 aria-expanded:text-background disabled:opacity-40"
  const selection = (
    <div
      className="relative mx-2.5 mb-2.5 flex shrink-0 items-center gap-0.5 rounded-xl bg-foreground p-1.5 text-background shadow-[0_12px_32px_rgb(0_0_0/0.18)]"
      role="toolbar"
      aria-label={t`Selection`}
      data-testid="selection-bar"
    >
      <Button
        variant="ghost"
        className={cn(bb, "w-[34px] px-0")}
        onClick={stopSelecting}
        aria-label={t`Cancel`}
        title={t`Clear the selection (Esc)`}
        data-testid="cancel-selection"
      >
        <XIcon />
      </Button>
      <span className="me-auto min-w-0 truncate pe-1.5 text-small font-semibold" aria-live="polite">
        {none ? <Trans>Pick conversations</Trans> : <Trans>{picked} selected</Trans>}
      </span>
      <Button
        variant="ghost"
        className={cn(bb, "w-[34px] shrink-0 px-0", allPicked && "bg-background/15")}
        disabled={rows.length === 0}
        onClick={() => setSelected(allPicked ? new Set() : new Set(rows.map((c) => c.id)))}
        aria-pressed={allPicked}
        aria-label={t`Select all`}
        title={t`Select all`}
        data-testid="select-all"
      >
        <ListChecksIcon />
      </Button>
      {inTrash ? (
        <>
          <Button
            variant="ghost"
            className={cn(bb, "shrink-0")}
            disabled={none || bulk.isPending}
            onClick={restoreChosen}
            data-testid="restore-selected"
          >
            <Trans>Restore</Trans>
          </Button>
          {canManage && (
            <DropdownMenu>
              <DropdownMenuTrigger
                render={<Button variant="ghost" className={cn(bb, "w-[34px] shrink-0 px-0")} disabled={none} />}
                aria-label={t`More`}
                title={t`More`}
                data-testid="more-selected"
              >
                <EllipsisVerticalIcon />
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="min-w-52">
                <DropdownMenuItem
                  variant="destructive"
                  disabled={trashTools.pending}
                  onClick={deleteChosen}
                  title={t`Delete for good (${SHORTCUTS.deleteForever})`}
                  data-testid="delete-forever-selected"
                >
                  <Trans>Delete forever</Trans>
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          )}
        </>
      ) : inSpam ? (
        <>
          <Button
            variant="ghost"
            className={cn(bb, "shrink-0")}
            disabled={none || bulk.isPending}
            onClick={notSpamChosen}
            title={t`Not spam (${SHORTCUTS.spam})`}
            data-testid="not-spam-selected"
          >
            <Trans>Not spam</Trans>
          </Button>
          {canManage && (
            <DropdownMenu>
              <DropdownMenuTrigger
                render={<Button variant="ghost" className={cn(bb, "w-[34px] shrink-0 px-0")} disabled={none} />}
                aria-label={t`More`}
                title={t`More`}
                data-testid="more-selected"
              >
                <EllipsisVerticalIcon />
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="min-w-52">
                <DropdownMenuItem
                  disabled={bulk.isPending}
                  onClick={trashChosen}
                  title={t`Move to the Trash (${SHORTCUTS.trash})`}
                  data-testid="trash-selected"
                >
                  <Trans>Move to the Trash</Trans>
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          )}
        </>
      ) : (
        <>
          <Button
            variant="ghost"
            className={cn(bb, "shrink-0")}
            disabled={none || bulk.isPending}
            onClick={closeChosen}
            data-testid="close-selected"
          >
            <Trans>Close</Trans>
          </Button>
          <DropdownMenu>
            <DropdownMenuTrigger
              render={<Button variant="ghost" className={cn(bb, "w-[34px] shrink-0 px-0")} disabled={none} />}
              aria-label={t`More`}
              title={t`More`}
              data-testid="more-selected"
            >
              <EllipsisVerticalIcon />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="min-w-52">
              <DropdownMenuItem onClick={readChosen} data-testid="read-selected">
                {anyUnread ? <Trans>Mark as read</Trans> : <Trans>Mark as unread</Trans>}
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuSub>
                <DropdownMenuSubTrigger data-testid="label-selected" disabled={bulk.isPending}>
                  <span className="flex-1">
                    <Trans>Label</Trans>
                  </span>
                </DropdownMenuSubTrigger>
                <DropdownMenuSubContent className="max-h-[60svh] min-w-48 overflow-y-auto">
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
                </DropdownMenuSubContent>
              </DropdownMenuSub>
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
                <DropdownMenuShortcut>{linkKey}</DropdownMenuShortcut>
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
                  </DropdownMenuItem>
                </>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        </>
      )}
    </div>
  )

  const chips = (
    <div className="flex shrink-0 flex-wrap items-center gap-1.5 px-3 pb-2.5" data-testid="chips">
      <InboxChip value={sidebarInbox || chipInbox} locked={locked} onChange={setInbox} />
      <SourceFilter inboxId={inboxId} onChange={reset} placeholder={<Trans>Filter</Trans>} />
      <LabelChip
        value={labelId}
        onChange={(id) => {
          setLabelId(id)
          reset()
        }}
      />
      <TeamChip
        value={teamId}
        onChange={(id) => {
          setTeamId(id)
          reset()
        }}
      />
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
    </div>
  )

  const counts = useCounts(["conversation"]).data
  const unassignedHere = useLoadedCount({
    kind: ["conversation"],
    purpose: "support",
    inbox_id: inboxId || undefined,
    status: ["open"],
    assignee: "unassigned",
  })
  const waitingHere = useLoadedCount(
    { kind: ["conversation"], purpose: "support", inbox_id: inboxId || undefined, status: ["open"] },
    isWaiting
  )

  return (
    <nav
      aria-label={t`Conversations`}
      className="@container/list flex h-full min-h-0 flex-col bg-surface"
      data-testid="chat-list"
    >
      {embedded && head}
      {!embedded && (
        <PhoneHeader>
          <ViewChips
            value={view.id ? null : chip}
            onChange={setChip}
            label={t`Filter`}
            items={tabs.map((k) => ({
              value: k,
              label: labels[k],
              count:
                k === "waiting"
                  ? waitingHere
                  : k === "unassigned"
                    ? inboxId
                      ? unassignedHere
                      : counts?.unassigned
                    : k === "teams"
                      ? counts?.mine_teams
                      : undefined,
              testId: `chip-${k}`,
            }))}
          />
        </PhoneHeader>
      )}
      {searchRow}
      {chips}
      <AwayBanner />
      {!view.id && chip === "waiting" && <CleanupBanner inboxId={inboxId} />}
      {inTrash && canManage && <TrashStrip onEmpty={(n) => trashTools.emptyTrash(n)} busy={trashTools.pending} />}
      {trashTools.dialogs}
      <ImportBanner />
      <div ref={scroller} className="min-h-0 flex-1 overflow-y-auto pb-3" data-testid="list">
        {list.isPending ? (
          <div className="grid gap-1 p-3">
            {[0, 1, 2, 3].map((i) => (
              <div key={i} className="flex items-center gap-3 py-1.5">
                <Skeleton className="size-9 rounded-full" />
                <div className="flex flex-1 flex-col gap-2">
                  <Skeleton className="h-4 w-1/2" />
                  <Skeleton className="h-3 w-3/4" />
                </div>
              </div>
            ))}
          </div>
        ) : rows.length === 0 && !hasNextPage ? (
          <div className="flex flex-col items-start gap-3 px-6 py-10" data-testid="list-empty">
            <p className="text-body text-muted-foreground">
              {q ? (
                <Trans>Nothing matches your search here.</Trans>
              ) : narrowed || unreadOnly ? (
                <Trans>Nothing matches these filters.</Trans>
              ) : (
                empty[view.id ? "all" : chip]
              )}
            </p>
            {filtered && (
              <Button
                variant="outline"
                className="h-8 rounded-[9px] border-input bg-card px-3 font-normal"
                onClick={clearFilters}
                data-testid="clear-filters"
              >
                <Trans>Clear search and filters</Trans>
              </Button>
            )}
          </div>
        ) : (
          <>
            {pinnedRows.length > 0 && (
              <div className={cn(look.over, "flex items-center gap-1.5 px-5 pt-3 pb-1.5")} data-testid="pinned-heading">
                <PinIcon className="size-3 rotate-45" />
                <Trans>Pinned</Trans>
              </div>
            )}
            {pinnedRows.map((c) => (
              <Row
                key={c.id}
                c={c}
                current={c.id === currentId}
                cursor={c.id === cursorId}
                live={live.has(c.id)}
                selecting={selecting}
                checked={selected.has(c.id)}
                onOpen={(range) => (selecting ? toggle(c.id, range) : onOpen(c.id))}
              />
            ))}
            {pinnedRows.length > 0 && mainRows.length > 0 && <div className="h-2" role="separator" />}
            {mainRows.map((c) => (
              <Row
                key={c.id}
                c={c}
                current={c.id === currentId}
                cursor={c.id === cursorId}
                live={live.has(c.id)}
                selecting={selecting}
                checked={selected.has(c.id)}
                onOpen={(range) => (selecting ? toggle(c.id, range) : onOpen(c.id))}
              />
            ))}
          </>
        )}
        <div ref={sentinel} className="h-px" />
        {isFetchingNextPage && (
          <div className="flex items-center gap-3 px-3 py-2">
            <Skeleton className="size-9 rounded-full" />
            <Skeleton className="h-4 w-1/2" />
          </div>
        )}
        <ErrorLine error={isSearchError(list.error) ? null : list.error} className="px-4 py-2" />
      </div>
      {selecting && selection}
      {confirmDialog}
      <BulkFailuresDialog failures={failures} rows={rows} onClose={() => setFailures([])} />
      {linking && (
        <LinkIncidentDialog conversationIds={chosen} onClose={() => setLinking(false)} onDone={stopSelecting} />
      )}
    </nav>
  )
}
