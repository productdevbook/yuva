import { Trans, useLingui } from "@lingui/react/macro"
import { useInfiniteQuery } from "@tanstack/react-query"
import { ScrollTextIcon, TerminalIcon } from "lucide-react"
import { useEffect, useState } from "react"
import { Navigate } from "react-router"

import { ErrorLine } from "@/components/common"
import { BotAvatar, MemberAvatar } from "@/components/common/Avatar"
import { EmptyBox } from "@/components/common/EmptyBox"
import { formatDateTime } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { useNear } from "@/hooks/use-near"
import { AUDIT_GROUPS, useAuditSentence, type AuditGroup } from "@/features/settings/audit/sentence"
import { GeneralHeader } from "@/features/settings/GeneralTabs"
import { Card, ChoiceSelect, Field, Rows } from "@/features/settings/ui"
import { api, unwrap, type AuditEntry } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { useMembers } from "@/lib/workspace"

const PAGE = 50

function dayStart(day: string, plusDays = 0) {
  const d = new Date(`${day}T00:00:00`)
  d.setDate(d.getDate() + plusDays)
  return d.toISOString()
}

function Actor({ entry }: { entry: AuditEntry }) {
  const { t } = useLingui()
  const { actor } = entry
  const name = actor.name ?? ""
  if (actor.kind === "system") {
    return (
      <span className="flex min-w-0 items-center gap-2">
        <span className="inline-flex size-[26px] shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground">
          <TerminalIcon className="size-3.5" />
        </span>
        <span className="truncate text-body font-medium">
          <Trans>System</Trans>
        </span>
      </span>
    )
  }
  if (actor.kind === "operator") {
    return (
      <span className="flex min-w-0 items-center gap-2">
        <span className="inline-flex size-[26px] shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground">
          <TerminalIcon className="size-3.5" />
        </span>
        <span className="truncate text-body font-medium">
          <Trans>Operator</Trans>
        </span>
      </span>
    )
  }
  const via = actor.via ?? ""
  const caption =
    actor.kind === "api_key" ? t`API key` : actor.kind === "oauth_grant" && via ? t`via ${via}` : undefined
  return (
    <span className="flex min-w-0 items-center gap-2">
      {actor.kind === "api_key" ? <BotAvatar name={name} /> : <MemberAvatar name={name} />}
      <span className="flex min-w-0 flex-col">
        <span className="truncate text-body font-medium">{name || actor.kind}</span>
        {caption && <span className="truncate text-caption text-faint">{caption}</span>}
      </span>
    </span>
  )
}

function Filters({
  actor,
  setActor,
  group,
  setGroup,
  from,
  setFrom,
  to,
  setTo,
}: {
  actor: string
  setActor: (v: string) => void
  group: AuditGroup | "all"
  setGroup: (v: AuditGroup | "all") => void
  from: string
  setFrom: (v: string) => void
  to: string
  setTo: (v: string) => void
}) {
  const { t } = useLingui()
  const members = useMembers().data ?? []
  const actors: [string, string][] = [
    ["all", t`Everyone`],
    ...members.map((m): [string, string] => [`member:${m.id}`, m.name || m.email]),
    ["kind:api_key", t`API keys`],
    ["kind:oauth_grant", t`Connected apps`],
    ["kind:operator", t`Operator`],
    ["kind:system", t`System`],
  ]
  const groups: [AuditGroup | "all", string][] = [
    ["all", t`Everything`],
    ["people", t`Members and invitations`],
    ["access", t`Keys and connected apps`],
    ["webhooks", t`Webhooks`],
    ["mail", t`Channels and inboxes`],
    ["data", t`Contacts and exports`],
    ["workspace", t`Workspace and broadcasts`],
  ]
  return (
    <div className="grid grid-cols-4 gap-3 phone:grid-cols-2" data-testid="audit-filters">
      <Field label={<Trans>Who</Trans>} htmlFor="audit-actor">
        <ChoiceSelect id="audit-actor" value={actor} onChange={setActor} options={actors} />
      </Field>
      <Field label={<Trans>What</Trans>} htmlFor="audit-group">
        <ChoiceSelect id="audit-group" value={group} onChange={setGroup} options={groups} />
      </Field>
      <Field label={<Trans>From</Trans>} htmlFor="audit-from">
        <Input
          id="audit-from"
          type="date"
          value={from}
          max={to || undefined}
          onChange={(e) => setFrom(e.target.value)}
        />
      </Field>
      <Field label={<Trans>To</Trans>} htmlFor="audit-to">
        <Input id="audit-to" type="date" value={to} min={from || undefined} onChange={(e) => setTo(e.target.value)} />
      </Field>
    </div>
  )
}

export function AuditLogPage() {
  const { isOwner, workspaceId: ws } = useSession()
  const { i18n } = useLingui()
  const sentence = useAuditSentence()
  const [actor, setActor] = useState("all")
  const [group, setGroup] = useState<AuditGroup | "all">("all")
  const [from, setFrom] = useState("")
  const [to, setTo] = useState("")
  const [end, setEnd] = useState<HTMLElement | null>(null)
  const filtered = actor !== "all" || group !== "all" || from !== "" || to !== ""
  const list = useInfiniteQuery({
    queryKey: keys.auditLog(ws, [actor, group, from, to].join("|")),
    queryFn: ({ pageParam }) => {
      const [kind, id] = actor.split(":")
      return unwrap(
        api.GET("/v1/audit-log", {
          params: {
            query: {
              cursor: pageParam,
              limit: PAGE,
              ...(kind === "member" ? { actor_id: id } : {}),
              ...(kind === "kind" ? { actor_kind: id as AuditEntry["actor"]["kind"] } : {}),
              ...(group !== "all" ? { action: AUDIT_GROUPS[group] } : {}),
              ...(from ? { since: dayStart(from) } : {}),
              ...(to ? { until: dayStart(to, 1) } : {}),
            },
          },
        })
      )
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor,
    enabled: isOwner,
  })
  const near = useNear(end, !!list.hasNextPage && !list.isFetchingNextPage)
  const { hasNextPage, isFetchingNextPage, error, fetchNextPage } = list
  useEffect(() => {
    if (near && hasNextPage && !isFetchingNextPage && !error) void fetchNextPage()
  }, [near, hasNextPage, isFetchingNextPage, error, fetchNextPage])
  if (!isOwner) return <Navigate to="/settings/profile" replace />
  const items = list.data?.pages.flatMap((p) => p.items) ?? []
  return (
    <>
      <GeneralHeader current="audit-log" />
      <p className="max-w-2xl text-muted-foreground">
        <Trans>
          Who changed members, access, keys, webhooks, mail accounts, retention and exports, newest first. Only owners
          see this.
        </Trans>
      </p>
      <Filters
        actor={actor}
        setActor={setActor}
        group={group}
        setGroup={setGroup}
        from={from}
        setFrom={setFrom}
        to={to}
        setTo={setTo}
      />
      {list.isPending ? (
        <Skeleton className="h-64 rounded-xl" />
      ) : items.length === 0 ? (
        <EmptyBox icon={ScrollTextIcon} title={<Trans>Nothing here</Trans>} testId="audit-empty">
          {filtered ? (
            <Trans>No entry matches these filters.</Trans>
          ) : (
            <Trans>Changes to members, keys, webhooks and other sensitive settings show up here.</Trans>
          )}
        </EmptyBox>
      ) : (
        <Card flush data-testid="audit-log">
          <Rows>
            {items.map((e) => (
              <li
                key={e.id}
                className="grid grid-cols-[150px_minmax(0,220px)_minmax(0,1fr)] items-center gap-x-4 gap-y-1 px-4 py-3 phone:grid-cols-1"
                data-testid="audit-row"
              >
                <time dateTime={e.created_at} className="text-small text-faint">
                  {formatDateTime(e.created_at, i18n.locale)}
                </time>
                <Actor entry={e} />
                <p className="min-w-0 text-body break-words">{sentence(e)}</p>
              </li>
            ))}
          </Rows>
          <div ref={setEnd} className="flex justify-center px-3 py-2 empty:hidden">
            {list.hasNextPage && (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => void list.fetchNextPage()}
                disabled={list.isFetchingNextPage}
              >
                <Trans>Load more</Trans>
              </Button>
            )}
          </div>
        </Card>
      )}
      <ErrorLine error={list.error} />
    </>
  )
}
