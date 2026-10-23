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
import { FilterChip } from "@/features/inbox/ListHead"
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
    <li className="flex gap-3.5 rounded-xl border bg-background p-4" data-testid="draft-row">
      <ContactAvatar id={item.conversation.contact.id} name={contact} className="size-9 font-semibold" />
      <div className="flex min-w-0 flex-1 flex-col gap-2">
        <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5">
          <Link to={to} className="min-w-0 truncate font-semibold outline-none hover:underline focus-visible:underline">
            {subject ? `${contact} · ${subject}` : contact}
          </Link>
          <ProductTag id={item.conversation.inbox_id} />
          <span className="flex-1" />
          <Ago iso={d.created_at} className={look.faint} />
        </div>
        <div className="flex flex-col gap-1.5 rounded-xl border border-dashed border-brand bg-brand-wash px-3.5 py-3">
          <span className="flex items-center gap-1.5 text-caption font-semibold text-brand">
            <SparklesIcon className="size-3.5" />
            {d.kind === "forward" ? <Trans>Forward draft by {author}</Trans> : <Trans>Draft by {author}</Trans>}
            <KindTag kind={d.author.type === "bot" ? "bot" : d.author.via ? "assistant" : "member"} />
          </span>
          <p className={cn("whitespace-pre-line", long && !open && "line-clamp-2")}>
            {d.kind === "forward" ? [d.forward?.to.join(", "), d.body].filter(Boolean).join("\n") : d.body}
          </p>
          {long && (
            <button
              type="button"
              aria-expanded={open}
              onClick={() => setOpen(!open)}
              className="w-fit text-caption font-medium text-brand outline-none hover:underline focus-visible:underline"
            >
              {open ? <Trans>Show less</Trans> : <Trans>Show full draft</Trans>}
            </button>
          )}
        </div>
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
          <Button className={look.btnPSm} render={<Link to={to} />} data-testid="draft-open">
            <Trans>Review draft</Trans>
          </Button>
          <span className={look.faint}>
            <Trans>Opens the conversation with the draft in the composer</Trans>
          </span>
        </div>
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
      <DialogContent data-testid="connect-assistant">
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
        <ul className="flex flex-col divide-y rounded-xl border">
          {ASSISTANTS.map((a) => {
            const Icon = a.icon
            const connected = grantsFor(a, grants, membership.member_id, new Set()).length > 0
            return (
              <li key={a.id} className="flex items-center gap-3 px-3 py-2.5">
                <span className="grid size-8 shrink-0 place-items-center rounded-lg border bg-surface text-muted-foreground">
                  <Icon className="size-4" />
                </span>
                <span className="min-w-0 flex-1 truncate font-medium">{nameOf(a)}</span>
                {connected && (
                  <span className="inline-flex items-center gap-1 text-caption font-medium text-success">
                    <CheckIcon className="size-3.5" />
                    <Trans>Connected</Trans>
                  </span>
                )}
                <Button
                  variant="outline"
                  className={look.btnSm}
                  render={<Link to={`/settings/connected-apps/${a.id}`} />}
                  onClick={onClose}
                >
                  {connected ? <Trans>Manage</Trans> : <Trans>Set up</Trans>}
                </Button>
              </li>
            )
          })}
        </ul>
      </DialogContent>
    </Dialog>
  )
}

function AppRow({ g, onRevoke }: { g: OAuthGrant; onRevoke: () => void }) {
  const { i18n } = useLingui()
  const members = useMemberMap()
  const { membership } = useSession()
  const a = ASSISTANTS.find((x) => x.matches(g))
  const Icon = a?.icon ?? PlugIcon
  const used = g.last_used_at ? formatRelative(g.last_used_at, i18n.locale) : null
  const since = formatDate(g.created_at, i18n.locale)
  const member = members.get(g.member_id)
  const by = g.member_id === membership.member_id ? null : member ? member.name || member.email : null
  const all = API_KEY_SCOPES.every((s) => g.scopes.includes(s))
  const count = g.scopes.length
  return (
    <li className="flex items-start gap-3 px-4 py-3.5" data-testid="assistant-grant-row">
      <span className="mt-0.5 grid size-9 shrink-0 place-items-center rounded-full border text-faint">
        <Icon className="size-4" />
      </span>
      <span className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="flex min-w-0 flex-wrap items-center gap-2">
          <span className="truncate font-medium">{g.client.name}</span>
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
          {" · "}
          {all ? <Trans>All scopes</Trans> : <Plural value={count} one="# scope" other="# scopes" />}
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
  const [confirm, confirmDialog] = useConfirm()
  const grants = useGrants()
  const revoke = useRevokeGrant()
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
      {grants.isPending && <Skeleton className="h-16 rounded-xl" />}
      <ErrorLine error={grants.error ?? revoke.error} />
      {grants.data && apps.length === 0 && (
        <EmptyBox
          icon={PlugIcon}
          title={<Trans>No assistant is connected yet</Trans>}
          testId="assistants-apps-empty"
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
        <ul className={cn(look.card, "divide-y")} data-testid="assistant-grants">
          {apps.map((g) => (
            <AppRow
              key={g.id}
              g={g}
              onRevoke={() => {
                const clientName = g.client.name
                confirm({
                  title: <Trans>Revoke {clientName}?</Trans>,
                  description: <Trans>Its access ends at once. To use it again, connect it again.</Trans>,
                  confirm: <Trans>Revoke</Trans>,
                  run: () => revoke.mutate(g.id),
                })
              }}
            />
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
              className="text-body text-muted-foreground outline-none hover:text-foreground focus-visible:underline"
            >
              <Trans>All assistants</Trans>
            </button>
          </div>
          <ul className="grid grid-cols-4 gap-2 max-lg:grid-cols-2">
            {others.map((a: Assistant) => {
              const Icon = a.icon
              return (
                <li key={a.id}>
                  <Link
                    to={`/settings/connected-apps/${a.id}`}
                    className="flex items-center gap-2 rounded-xl border px-3 py-2.5 outline-none transition-colors hover:border-faint focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    <Icon className="size-4 shrink-0 text-muted-foreground" />
                    <span className="truncate">{nameOf(a)}</span>
                  </Link>
                </li>
              )
            })}
          </ul>
        </section>
      )}
      {confirmDialog}
    </>
  )
}

export function AssistantsPage() {
  const { t } = useLingui()
  const [inboxId] = useInboxFilter()
  const [tab, setTab] = useState<AssistantTab>("drafts")
  const [author, setAuthor] = useState<AuthorChip>("all")
  const [connect, setConnect] = useState(false)
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
  const grants = useGrants()
  const workspace = useWorkspaceSettings()
  const chips: [AuthorChip, string][] = [
    ["all", t`All`],
    ["bot", t`Bots`],
    ["assistant", t`Assistants`],
    ["member", t`Members`],
  ]
  return (
    <main className="flex min-h-0 flex-1 flex-col" data-testid="assistants-panel">
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
      <PageBody>
        <div className="flex max-w-[908px] flex-col gap-3.5 p-6 phone:px-4">
          {tab === "drafts" ? (
            <>
              {workspace.data && (
                <div
                  className="flex flex-wrap items-center gap-x-2.5 gap-y-1 rounded-xl bg-surface px-3.5 py-3 text-muted-foreground"
                  data-testid="drafts-note"
                >
                  <SparklesIcon className="size-4 shrink-0" />
                  <span className="min-w-0 flex-1 basis-64">
                    {workspace.data.bots_may_send ? (
                      <Trans>
                        Assistants and API keys may send replies themselves: <b>Bots may send</b> is on in this
                        workspace.
                      </Trans>
                    ) : (
                      <Trans>
                        Assistants and API keys write drafts; a member reviews and sends them. <b>Bots may send</b> is
                        off in this workspace.
                      </Trans>
                    )}
                  </span>
                  <Link
                    to="/settings/api-keys"
                    className="text-foreground underline-offset-4 hover:underline"
                    data-testid="drafts-note-link"
                  >
                    <Trans>Change in API & MCP</Trans>
                  </Link>
                </div>
              )}
              <div
                className="flex flex-wrap gap-1.5"
                role="toolbar"
                aria-label={t`Draft author`}
                data-testid="draft-chips"
              >
                {chips.map(([k, label]) => (
                  <FilterChip key={k} active={author === k} onClick={() => setAuthor(k)} testId={`draft-chip-${k}`}>
                    {label}
                    <span className="tabular-nums opacity-70">{count(k)}</span>
                  </FilterChip>
                ))}
                <span className="sr-only" data-testid="drafts-total">
                  {total}
                </span>
              </div>
              {drafts.isPending && <Skeleton className="h-32 rounded-xl" />}
              <ErrorLine error={drafts.error} />
              {drafts.data && items.length === 0 && author === "all" && (
                <EmptyBox icon={CheckIcon} title={<Trans>No drafts are waiting</Trans>} testId="drafts-empty">
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
                  actions={
                    <Button variant="outline" className={look.btn} onClick={() => setAuthor("all")}>
                      <Trans>Show all authors</Trans>
                    </Button>
                  }
                >
                  <Trans>Nothing from this kind of author is waiting for review.</Trans>
                </EmptyBox>
              )}
              <ul className="flex flex-col gap-4" data-testid="draft-list">
                {items.map((x) => (
                  <DraftCard key={x.draft.id} item={x} />
                ))}
              </ul>
              <MoreButton query={drafts} />
            </>
          ) : (
            <Apps onConnect={() => setConnect(true)} />
          )}
        </div>
      </PageBody>
      {connect && <ConnectDialog grants={grants.data ?? []} onClose={() => setConnect(false)} />}
    </main>
  )
}
