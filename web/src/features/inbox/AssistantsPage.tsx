import { Plural, Select, Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, PlugIcon, SparklesIcon } from "lucide-react"
import { useState } from "react"
import { Link } from "react-router"

import { ContactAvatar, ErrorLine, useConfirm } from "@/components/common"
import { EmptyBox } from "@/components/common/EmptyBox"
import { CopyButton } from "@/components/common/CopyButton"
import { API_KEY_SCOPES, formatDate, formatRelative } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { InboxPicker } from "@/features/inbox/InboxPicker"
import { Ago, FrameTabs, look, PageBody, PageTop } from "@/features/inbox/PageFrame"
import { ProductTag } from "@/features/inbox/ProductTag"
import { useDrafts } from "@/features/inbox/queries"
import { messageLink, MoreButton, useAuthorLabel } from "@/features/inbox/SidePanels"
import {
  ASSISTANTS,
  grantsFor,
  mcpUrl,
  useAssistantName,
  type Assistant,
} from "@/features/settings/connected-apps/assistants"
import { useGrants, useRevokeGrant, useWorkspaceSettings } from "@/features/settings/connected-apps/queries"
import { LoadProblem } from "@/features/reports/parts"
import type { DraftAuthorKind, DraftItem, OAuthGrant } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useMemberMap } from "@/lib/workspace"

type AuthorChip = "all" | DraftAuthorKind
type AssistantTab = "drafts" | "apps"

function KindTag({ kind }: { kind: DraftAuthorKind }) {
  return (
    <span className="rounded-[4px] border px-1.5 text-[11px] leading-4 font-medium text-muted-foreground">
      {kind === "assistant" ? <Trans>Assistant</Trans> : kind === "bot" ? <Trans>Bot</Trans> : <Trans>Member</Trans>}
    </span>
  )
}

function DraftCard({ item }: { item: DraftItem }) {
  const label = useAuthorLabel()
  const [open, setOpen] = useState(false)
  const d = item.draft
  const author = label(d.author)
  const contact = item.conversation.contact.name || item.conversation.contact.email || ""
  const subject = item.conversation.subject
  const to = messageLink(item.conversation.id, d.id)
  const long = d.body.length > 150 || d.body.split("\n").length > 2
  return (
    <li className="flex flex-col gap-2.5 rounded-[14px] border bg-card p-4" data-testid="draft-row">
      <div className="flex items-start gap-3">
        <ContactAvatar
          id={item.conversation.contact.id}
          name={contact}
          className="size-[34px] text-[11px] font-semibold"
        />
        <div className="flex min-w-0 flex-1 flex-col gap-0.5">
          <Link
            to={to}
            className="min-w-0 truncate text-[14px] leading-5 font-semibold outline-none hover:underline focus-visible:underline"
          >
            {subject || contact}
          </Link>
          <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5 text-caption text-faint">
            {subject && <span className="max-w-full truncate">{contact}</span>}
            <ProductTag id={item.conversation.inbox_id} />
          </div>
        </div>
        <Ago iso={d.created_at} className={cn(look.faint, "whitespace-nowrap tabular-nums")} />
      </div>
      <div className="relative overflow-hidden rounded-[11px] border border-dashed border-input bg-muted px-3.5 py-3">
        <p className={cn("text-[13.5px] leading-[21px] whitespace-pre-line", long && !open && "line-clamp-2")}>
          {d.kind === "forward" ? [d.forward?.to.join(", "), d.body].filter(Boolean).join("\n") : d.body}
        </p>
        {long && !open && (
          <span
            aria-hidden
            className="pointer-events-none absolute inset-x-0 bottom-0 h-7 bg-gradient-to-b from-transparent to-muted"
          />
        )}
      </div>
      <div className="flex flex-wrap items-center gap-x-2 gap-y-2">
        <span className="inline-flex min-w-0 items-center gap-1.5 rounded-full bg-muted py-0.5 pr-2.5 pl-2 text-caption font-medium text-foreground/80">
          <SparklesIcon className="size-3.5 shrink-0 text-brand" />
          <span className="truncate">
            {d.kind === "forward" ? <Trans>Forward draft by {author}</Trans> : <Trans>Draft by {author}</Trans>}
          </span>
          <KindTag kind={d.author.type === "bot" ? "bot" : d.author.via ? "assistant" : "member"} />
        </span>
        {long && (
          <button
            type="button"
            aria-expanded={open}
            onClick={() => setOpen(!open)}
            className="h-7 rounded-md px-2 text-body font-medium outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
          >
            {open ? <Trans>Show less</Trans> : <Trans>Show full draft</Trans>}
          </button>
        )}
        <span className="flex-1" />
        <Button
          className={look.btnInkSm}
          render={<Link to={to} />}
          data-testid="draft-open"
          aria-describedby={`draft-hint-${d.id}`}
        >
          <Trans>Review draft</Trans>
        </Button>
        <span id={`draft-hint-${d.id}`} className="sr-only">
          <Trans>Opens the conversation with the draft in the composer</Trans>
        </span>
      </div>
    </li>
  )
}

function ConnectDialog({ grants, onClose }: { grants: OAuthGrant[]; onClose: () => void }) {
  const { membership } = useSession()
  const nameOf = useAssistantName()
  const url = mcpUrl()
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-[600px]" data-testid="connect-assistant">
        <DialogHeader>
          <DialogTitle>
            <Trans>Connect an assistant</Trans>
          </DialogTitle>
          <DialogDescription>
            <Trans>Pick the one you use. Setup steps and scopes are on API & MCP.</Trans>
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="mcp-server-url">
            <Trans>Server URL</Trans>
          </Label>
          <div className="flex gap-2">
            <Input id="mcp-server-url" readOnly value={url} className="min-w-0 flex-1 font-mono" dir="ltr" />
            <CopyButton value={url} />
          </div>
        </div>
        <ul className="grid grid-cols-2 gap-2 max-sm:grid-cols-1">
          {ASSISTANTS.map((a) => {
            const Icon = a.icon
            const connected = grantsFor(a, grants, membership.member_id, new Set()).length > 0
            return (
              <li key={a.id}>
                <Link
                  to={`/settings/connected-apps/${a.id}`}
                  onClick={onClose}
                  className="flex items-center gap-2.5 rounded-[11px] border bg-card px-3 py-2.5 outline-none transition-colors hover:border-faint focus-visible:ring-2 focus-visible:ring-ring"
                >
                  <AppTile icon={Icon} className="size-8 rounded-[9px]" />
                  <span className="flex min-w-0 flex-1 flex-col">
                    <span className="truncate font-semibold">{nameOf(a)}</span>
                    <span
                      className={cn(
                        "inline-flex items-center gap-1 text-caption",
                        connected ? "font-medium text-success" : "text-faint"
                      )}
                    >
                      {connected && <CheckIcon className="size-3.5" />}
                      {connected ? <Trans>Connected</Trans> : <Trans>Set up</Trans>}
                    </span>
                  </span>
                </Link>
              </li>
            )
          })}
        </ul>
      </DialogContent>
    </Dialog>
  )
}

function AppTile({ icon: Icon, className }: { icon: React.ComponentType<{ className?: string }>; className?: string }) {
  return (
    <span className={cn("grid shrink-0 place-items-center bg-muted text-foreground/80", className)}>
      <Icon className="size-4" />
    </span>
  )
}

function ScopeChips({ g, max }: { g: OAuthGrant; max: number }) {
  const all = API_KEY_SCOPES.every((s) => g.scopes.includes(s))
  const shown = g.scopes.slice(0, max)
  const rest = g.scopes.length - shown.length
  const chip =
    "inline-flex h-[18px] max-w-full items-center rounded-[5px] border bg-surface px-1.5 font-mono text-[11px] text-foreground/80"
  return (
    <span className="flex flex-wrap gap-1">
      {all ? (
        <span className={chip}>
          <Trans>All scopes</Trans>
        </span>
      ) : (
        <>
          {shown.map((s) => (
            <span key={s} className={cn(chip, "truncate")}>
              {s}
            </span>
          ))}
          {rest > 0 && <span className={chip}>+{rest}</span>}
        </>
      )}
    </span>
  )
}

function useRevoker() {
  const [confirm, dialog] = useConfirm()
  const revoke = useRevokeGrant()
  const ask = (g: OAuthGrant) => {
    const clientName = g.client.name
    confirm({
      title: <Trans>Revoke {clientName}?</Trans>,
      description: <Trans>Its access ends at once. To use it again, connect it again.</Trans>,
      confirm: <Trans>Revoke</Trans>,
      run: () => revoke.mutate(g.id),
    })
  }
  return { ask, dialog, error: revoke.error }
}

function useUsage(g: OAuthGrant) {
  const { i18n } = useLingui()
  return g.last_used_at ? formatRelative(g.last_used_at, i18n.locale) : null
}

function AppRow({ g, onRevoke }: { g: OAuthGrant; onRevoke: () => void }) {
  const { i18n } = useLingui()
  const members = useMemberMap()
  const { membership } = useSession()
  const a = ASSISTANTS.find((x) => x.matches(g))
  const used = useUsage(g)
  const since = formatDate(g.created_at, i18n.locale)
  const member = members.get(g.member_id)
  const by = g.member_id === membership.member_id ? null : member ? member.name || member.email : null
  return (
    <li
      className="flex items-start gap-3 border-t px-4 py-3.5 first:border-t-0 @max-xl:flex-wrap"
      data-testid="assistant-grant-row"
    >
      <AppTile icon={a?.icon ?? PlugIcon} className="size-9 rounded-[10px]" />
      <span className="flex min-w-0 flex-1 basis-56 flex-col gap-0.5">
        <span className="flex min-w-0 flex-wrap items-center gap-2">
          <span className="truncate font-semibold">{g.client.name}</span>
          <span className="rounded-full border border-success/35 px-2 py-px text-caption font-medium text-success">
            {g.resource === "mcp" ? <Trans>MCP only</Trans> : <Trans>Full API</Trans>}
          </span>
        </span>
        <span className="text-caption text-muted-foreground">
          {used ? <Trans>Last used {used}</Trans> : <Trans>Not used yet</Trans>}
          {" · "}
          <Plural value={g.requests_this_month} one="# request this month" other="# requests this month" />
        </span>
        <span className="text-caption text-faint">
          {by ? (
            <Trans>
              Connected by {by} on {since}
            </Trans>
          ) : (
            <Trans>Connected on {since}</Trans>
          )}
        </span>
        <span className="mt-1">
          <ScopeChips g={g} max={6} />
        </span>
      </span>
      <span className="flex shrink-0 items-center gap-2">
        <Button
          variant="outline"
          className={look.btnSm}
          render={<Link to={a ? `/settings/connected-apps/${a.id}` : "/settings/connected-apps"} />}
        >
          <Trans>Manage</Trans>
        </Button>
        <Button variant="ghost" className="h-[30px] rounded-lg px-2.5 text-destructive" onClick={onRevoke}>
          <Trans>Revoke</Trans>
        </Button>
      </span>
    </li>
  )
}

function Apps({ onConnect }: { onConnect: () => void }) {
  const { membership } = useSession()
  const nameOf = useAssistantName()
  const grants = useGrants()
  const revoker = useRevoker()
  const apps = grants.data ?? []
  const others = ASSISTANTS.filter((a) => grantsFor(a, apps, membership.member_id, new Set()).length === 0)
  return (
    <>
      <p className="text-muted-foreground">
        <Trans>
          Assistants work in Yuva through MCP, acting as the member who connected them, with only the access that member
          approved.
        </Trans>
      </p>
      {grants.isPending && (
        <div aria-busy="true" className="flex flex-col gap-2.5">
          <Skeleton className="h-24 rounded-[14px]" />
          <Skeleton className="h-24 rounded-[14px]" />
        </div>
      )}
      {grants.error ? (
        <LoadProblem error={grants.error} onRetry={() => void grants.refetch()} testId="assistants-apps-error" />
      ) : (
        <ErrorLine error={revoker.error} />
      )}
      {grants.data && apps.length === 0 && (
        <EmptyBox
          icon={PlugIcon}
          title={<Trans>No assistant is connected yet</Trans>}
          testId="assistants-apps-empty"
          className="rounded-[14px] bg-card"
          actions={
            <Button className={look.btnInk} onClick={onConnect}>
              <Trans>Connect an assistant</Trans>
            </Button>
          }
        >
          <Trans>
            Connect Claude, ChatGPT, Cursor or another MCP client to let it read conversations and draft replies for you
            to review.
          </Trans>
        </EmptyBox>
      )}
      {apps.length > 0 && (
        <ul className="overflow-hidden rounded-[14px] border bg-card" data-testid="assistant-grants">
          {apps.map((g) => (
            <AppRow key={g.id} g={g} onRevoke={() => revoker.ask(g)} />
          ))}
        </ul>
      )}
      {apps.length > 0 && others.length > 0 && (
        <section className="flex flex-col gap-2">
          <div className="flex items-center justify-between gap-3">
            <h2 className={look.over}>
              <Trans>Add another</Trans>
            </h2>
            <button
              type="button"
              onClick={onConnect}
              className="rounded-md px-2 py-1 text-body text-muted-foreground outline-none hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
            >
              <Trans>All assistants</Trans>
            </button>
          </div>
          <ul className="grid grid-cols-4 gap-2 @max-3xl:grid-cols-2 @max-sm:grid-cols-1">
            {others.map((a: Assistant) => {
              const Icon = a.icon
              return (
                <li key={a.id}>
                  <Link
                    to={`/settings/connected-apps/${a.id}`}
                    className="flex items-center gap-2.5 rounded-[11px] border bg-card px-3 py-2.5 outline-none transition-colors hover:border-faint focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    <AppTile icon={Icon} className="size-7 rounded-lg" />
                    <span className="truncate">{nameOf(a)}</span>
                  </Link>
                </li>
              )
            })}
          </ul>
        </section>
      )}
      {revoker.dialog}
    </>
  )
}

function ConnectionsCard({ onManage, onConnect }: { onManage?: () => void; onConnect: () => void }) {
  const grants = useGrants()
  const revoker = useRevoker()
  const apps = grants.data ?? []
  return (
    <section className="shrink-0 overflow-hidden rounded-[14px] border bg-card" data-testid="assistants-connections">
      <header className="flex items-center gap-3 px-4 pt-3.5 pb-2.5">
        <h2 className="flex-1 text-[14px] leading-5 font-semibold">
          <Trans>Connected apps</Trans>
        </h2>
        {apps.length > 0 && onManage && (
          <button
            type="button"
            onClick={onManage}
            className="h-7 rounded-md px-2 text-body font-medium outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
          >
            <Trans>Manage</Trans>
          </button>
        )}
      </header>
      {grants.isPending && (
        <div aria-busy="true" className="flex flex-col gap-3 px-4 pb-4">
          <Skeleton className="h-14 rounded-xl" />
          <Skeleton className="h-14 rounded-xl" />
        </div>
      )}
      {grants.error && (
        <div className="px-4 pb-4">
          <ErrorLine error={grants.error} />
          <Button variant="outline" className={cn(look.btnSm, "mt-2")} onClick={() => void grants.refetch()}>
            <Trans>Try again</Trans>
          </Button>
        </div>
      )}
      {grants.data && apps.length === 0 && (
        <div className="flex flex-col gap-2.5 px-4 pb-4" data-testid="assistants-connections-empty">
          <b className="font-semibold">
            <Trans>No assistant is connected yet</Trans>
          </b>
          <p className="text-muted-foreground">
            <Trans>
              Connect Claude, ChatGPT, Cursor or another MCP client to let it read conversations and draft replies for
              you to review.
            </Trans>
          </p>
          <div>
            <Button variant="outline" className={look.btn} onClick={onConnect}>
              <Trans>Connect an assistant</Trans>
            </Button>
          </div>
        </div>
      )}
      <ul>
        {apps.map((g) => (
          <ConnectionRow key={g.id} g={g} onRevoke={() => revoker.ask(g)} />
        ))}
      </ul>
      {apps.length > 0 && !onManage && (
        <div className="border-t px-4 py-3">
          <Button variant="outline" className={look.btn} onClick={onConnect}>
            <Trans>Connect an assistant</Trans>
          </Button>
        </div>
      )}
      <ErrorLine error={revoker.error} />
      {revoker.dialog}
    </section>
  )
}

function ConnectionRow({ g, onRevoke }: { g: OAuthGrant; onRevoke: () => void }) {
  const a = ASSISTANTS.find((x) => x.matches(g))
  const used = useUsage(g)
  return (
    <li className="flex items-start gap-3 border-t px-4 py-3.5" data-testid="assistant-connection">
      <AppTile icon={a?.icon ?? PlugIcon} className="size-9 rounded-[10px]" />
      <span className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="truncate font-semibold">{g.client.name}</span>
        <span className="text-caption text-faint">
          {used ? <Trans>Last used {used}</Trans> : <Trans>Not used yet</Trans>}
          {" · "}
          <Plural value={g.requests_this_month} one="# request this month" other="# requests this month" />
        </span>
        <span className="mt-1">
          <ScopeChips g={g} max={3} />
        </span>
      </span>
      <button
        type="button"
        onClick={onRevoke}
        className="h-7 shrink-0 rounded-md px-2 text-body font-medium text-destructive outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
      >
        <Trans>Revoke</Trans>
      </button>
    </li>
  )
}

function AuthorSeg({
  value,
  onChange,
  items,
  label,
}: {
  value: AuthorChip
  onChange: (k: AuthorChip) => void
  items: [AuthorChip, string, number][]
  label: string
}) {
  return (
    <div
      role="toolbar"
      aria-label={label}
      className="inline-flex max-w-full gap-0.5 overflow-x-auto rounded-[9px] bg-muted p-0.5 [scrollbar-width:none]"
      data-testid="draft-chips"
    >
      {items.map(([k, text, n]) => (
        <button
          key={k}
          type="button"
          aria-pressed={value === k}
          onClick={() => onChange(k)}
          data-testid={`draft-chip-${k}`}
          className={cn(
            "inline-flex h-[30px] shrink-0 items-center gap-1.5 rounded-[7px] px-2.5 text-small whitespace-nowrap outline-none focus-visible:ring-2 focus-visible:ring-ring",
            value === k
              ? "bg-card font-semibold text-foreground shadow-xs"
              : "text-muted-foreground hover:text-foreground"
          )}
        >
          {text}
          <span className="tabular-nums opacity-70">{n}</span>
        </button>
      ))}
    </div>
  )
}

function useDraftsView() {
  const { t, i18n } = useLingui()
  const [inboxId] = useInboxFilter()
  const [author, setAuthor] = useState<AuthorChip>("all")
  const filter = inboxId || undefined
  const all = useDrafts({ inbox_id: filter })
  const bots = useDrafts({ inbox_id: filter, author: "bot" })
  const assistants = useDrafts({ inbox_id: filter, author: "assistant" })
  const members = useDrafts({ inbox_id: filter, author: "member" })
  const byAuthor = { all, bot: bots, assistant: assistants, member: members }
  const drafts = byAuthor[author]
  const count = (k: AuthorChip) => Number(byAuthor[k].data?.pages[0]?.total ?? 0)
  const items = drafts.data?.pages.flatMap((p) => p.items) ?? []
  const total = count("all")
  const workspace = useWorkspaceSettings()
  const fmt = new Intl.NumberFormat(i18n.locale)
  const chips: [AuthorChip, string, number][] = [
    ["all", t`All`, count("all")],
    ["bot", t`Bots`, count("bot")],
    ["assistant", t`Assistants`, count("assistant")],
    ["member", t`Members`, count("member")],
  ]
  return { author, setAuthor, drafts, items, total, chips, fmt, workspace }
}

type DraftsView = ReturnType<typeof useDraftsView>

function DraftsColumn({ view }: { view: DraftsView }) {
  const { t } = useLingui()
  const { author, setAuthor, drafts, items, total, chips, fmt, workspace } = view
  return (
    <div className="flex min-w-0 flex-col gap-3">
      <div className="flex flex-wrap items-center gap-x-2.5 gap-y-2">
        <h2 className="text-[15px] leading-5 font-semibold">
          <Trans>Drafts waiting</Trans>
        </h2>
        {drafts.data && (
          <span className="min-w-[18px] rounded-full bg-note px-1.5 text-center text-[11px] leading-[18px] font-semibold text-note-ink tabular-nums">
            {fmt.format(total)}
          </span>
        )}
        <span className="flex-1" />
        <AuthorSeg value={author} onChange={setAuthor} items={chips} label={t`Draft author`} />
        <span className="sr-only" data-testid="drafts-total">
          {total}
        </span>
      </div>
      {workspace.data && (
        <div
          className="flex flex-wrap items-center gap-x-2.5 gap-y-1 rounded-[10px] bg-muted px-3 py-2.5 text-muted-foreground"
          data-testid="drafts-note"
        >
          <SparklesIcon className="size-4 shrink-0" />
          <span className="min-w-0 flex-1 basis-64">
            {workspace.data.bots_may_send ? (
              <Trans>
                Assistants and API keys may send replies themselves: <b>Bots may send</b> is on in this workspace.
              </Trans>
            ) : (
              <Trans>
                Assistants and API keys write drafts; a member reviews and sends them. <b>Bots may send</b> is off in
                this workspace.
              </Trans>
            )}
          </span>
          <Link
            to="/settings/api-keys"
            className="rounded-md text-foreground underline-offset-4 outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
            data-testid="drafts-note-link"
          >
            <Trans>Change in API & MCP</Trans>
          </Link>
        </div>
      )}
      {drafts.isPending && (
        <div aria-busy="true" className="flex flex-col gap-2.5">
          <Skeleton className="h-40 rounded-[14px]" />
          <Skeleton className="h-40 rounded-[14px]" />
          <Skeleton className="h-40 rounded-[14px]" />
        </div>
      )}
      {drafts.error && <LoadProblem error={drafts.error} onRetry={() => void drafts.refetch()} testId="drafts-error" />}
      {drafts.data && items.length === 0 && author === "all" && (
        <EmptyBox
          icon={CheckIcon}
          title={<Trans>No drafts are waiting</Trans>}
          testId="drafts-empty"
          className="rounded-[14px] bg-card"
        >
          <Trans>
            Replies an assistant, a bot or a teammate drafts show up here until someone sends or discards them.
          </Trans>
        </EmptyBox>
      )}
      {drafts.data && items.length === 0 && author !== "all" && (
        <EmptyBox
          icon={CheckIcon}
          title={
            <Select
              value={author}
              _bot="No drafts by bots"
              _assistant="No drafts by assistants"
              _member="No drafts by members"
              other="No drafts by this author"
            />
          }
          testId="drafts-empty-filtered"
          className="rounded-[14px] bg-card"
          actions={
            <Button variant="outline" className={look.btn} onClick={() => setAuthor("all")}>
              <Trans>Show all authors</Trans>
            </Button>
          }
        >
          <Trans>Nothing from this kind of author is waiting for review.</Trans>
        </EmptyBox>
      )}
      <ul className="flex flex-col gap-2.5" data-testid="draft-list">
        {items.map((x) => (
          <DraftCard key={x.draft.id} item={x} />
        ))}
      </ul>
      <MoreButton query={drafts} />
    </div>
  )
}

export function AssistantsPage() {
  const { t } = useLingui()
  const [tab, setTab] = useState<AssistantTab>("drafts")
  const [connect, setConnect] = useState(false)
  const view = useDraftsView()
  const { total } = view
  const grants = useGrants()
  return (
    <main className="@container flex min-h-0 flex-1 flex-col" data-testid="assistants-panel">
      <PageTop
        title={<Trans>Assistants</Trans>}
        sub={<Trans>Drafts waiting for review and the assistants you connected</Trans>}
        actions={
          <Button className={look.btnInk} onClick={() => setConnect(true)} data-testid="assistants-connect">
            <Trans>Connect</Trans>
          </Button>
        }
        tabs={
          <FrameTabs
            label={t`Show`}
            value={tab}
            onChange={setTab}
            items={[
              { key: "drafts", label: <Trans>Drafts waiting</Trans>, count: total, testId: "assistants-tab-drafts" },
              {
                key: "apps",
                label: <Trans>Connected apps</Trans>,
                count: grants.data?.length,
                testId: "assistants-tab-apps",
              },
            ]}
          />
        }
      />
      <PageBody className="bg-surface">
        {tab === "drafts" ? (
          <div className="mx-auto grid max-w-[1280px] grid-cols-[minmax(0,1fr)_380px] items-start gap-4 px-6 py-5 pb-12 @max-[1100px]:grid-cols-1 @max-xl:px-4">
            <DraftsColumn view={view} />
            <aside className="min-w-0 @min-[1100px]:sticky @min-[1100px]:top-4">
              <ConnectionsCard onManage={() => setTab("apps")} onConnect={() => setConnect(true)} />
            </aside>
          </div>
        ) : (
          <div className="mx-auto flex max-w-[908px] flex-col gap-3.5 px-6 py-5 pb-12 @max-xl:px-4">
            <Apps onConnect={() => setConnect(true)} />
          </div>
        )}
      </PageBody>
      {connect && <ConnectDialog grants={grants.data ?? []} onClose={() => setConnect(false)} />}
    </main>
  )
}

export function AssistantsPanel() {
  const view = useDraftsView()
  const [connect, setConnect] = useState(false)
  const grants = useGrants()
  return (
    <section className="@container flex h-full min-h-0 flex-col bg-background" data-testid="assistants-panel">
      <header className="flex shrink-0 flex-col gap-1 border-b px-4 pt-4 pb-3.5">
        <div className="flex min-h-8 min-w-0 items-center gap-2">
          <h1 className="min-w-0 flex-1 truncate text-title">
            <Trans>Assistants</Trans>
          </h1>
          <Button className={look.btnInk} onClick={() => setConnect(true)} data-testid="assistants-connect">
            <Trans>Connect</Trans>
          </Button>
        </div>
        <p className="text-small text-muted-foreground">
          <Trans>Drafts waiting for review and the assistants you connected</Trans>
        </p>
        <InboxPicker className="mt-1.5 max-w-full self-start" />
      </header>
      <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto bg-surface px-4 py-4 pb-8">
        <DraftsColumn view={view} />
        <ConnectionsCard onConnect={() => setConnect(true)} />
      </div>
      {connect && <ConnectDialog grants={grants.data ?? []} onClose={() => setConnect(false)} />}
    </section>
  )
}
