import "@/features/help/i18n"
import { Trans, useLingui } from "@lingui/react/macro"
import { PlusIcon } from "lucide-react"
import { useEffect, useRef, useState } from "react"
import { Link, useMatch, useNavigate } from "react-router"

import { Dot } from "@/components/common"
import { ColumnHeading } from "@/components/common/Column"
import { HELP_KEYS, keyLabel } from "@/components/common/ShortcutSheet"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { CreateCenterDialog } from "@/features/help/CreateCenterDialog"
import { centerLink, useHelpCenters } from "@/features/help/queries"
import { ErrorLine } from "@/features/help/text"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { InboxPicker } from "@/features/inbox/InboxPicker"
import { ListHead } from "@/features/inbox/ListHead"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useResetState } from "@/hooks/use-reset-state"
import type { HelpCenter, Inbox } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { isSupport, useInboxes } from "@/lib/workspace"

function CenterRow({ c, current, cursor }: { c: HelpCenter; current: boolean; cursor: boolean }) {
  const published = c.counts.published_articles
  const total = c.counts.articles
  return (
    <li>
      <Link
        to={centerLink(c.id)}
        className={cn(
          "flex flex-col gap-1 rounded-xl px-2.5 py-2.5 outline-none transition-colors hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring",
          current && "bg-muted",
          cursor && "ring-2 ring-ring"
        )}
        aria-current={current ? "page" : undefined}
        data-testid="help-row"
      >
        <span className="flex items-center gap-2">
          <span className="min-w-0 flex-1 truncate text-body font-medium">{c.name}</span>
          <span className="shrink-0 text-caption text-faint">
            {c.published ? <Trans>Published</Trans> : <Trans>Unpublished</Trans>}
          </span>
        </span>
        <span className="truncate text-caption text-muted-foreground">
          <Trans>
            {published} of {total} articles published
          </Trans>
        </span>
      </Link>
    </li>
  )
}

export default function HelpColumn() {
  const { t } = useLingui()
  const navigate = useNavigate()
  const { canManage } = useSession()
  const [inboxId] = useInboxFilter()
  const inboxQuery = useInboxes()
  const inboxes = (inboxQuery.data ?? []).filter(isSupport)
  const centers = useHelpCenters()
  const items = centers.data ?? []
  const shown = inboxId ? inboxes.filter((i) => i.id === inboxId) : inboxes
  const groups = shown.map((inbox) => ({ inbox, center: items.find((c) => c.inbox_id === inbox.id) }))
  const ordered = groups.flatMap((g) => (g.center ? [g.center] : []))
  const currentId = useMatch("/knowledge/:centerId/*")?.params.centerId
  const [creating, setCreating] = useState<Inbox | null>(null)
  const [cursor, setCursor] = useResetState(-1, inboxId)
  const list = useRef<HTMLDivElement>(null)
  const missing = canManage ? groups.find((g) => !g.center)?.inbox : undefined
  const newKey = keyLabel(HELP_KEYS.newCollection).join("")

  useHotkeys({
    j: () => setCursor((i) => Math.min(ordered.length - 1, i + 1)),
    k: () => setCursor((i) => Math.max(0, i - 1)),
    ArrowDown: () => setCursor((i) => Math.min(ordered.length - 1, i + 1)),
    ArrowUp: () => setCursor((i) => Math.max(0, i - 1)),
    ...(missing && !currentId ? { [HELP_KEYS.newCollection]: () => setCreating(missing) } : {}),
    ...(cursor >= 0 && ordered[cursor]
      ? {
          Enter: () => {
            navigate(centerLink(ordered[cursor].id))
            setCursor(-1)
          },
        }
      : {}),
  })
  useEffect(() => {
    if (cursor >= 0)
      list.current?.querySelectorAll("[data-testid=help-row]")[cursor]?.scrollIntoView({ block: "nearest" })
  }, [cursor])

  let index = 0
  return (
    <>
      <section className="flex h-full min-h-0 flex-col bg-background" data-testid="help-panel">
        <ListHead
          title={<Trans>Help center</Trans>}
          sub={<Trans>Articles people read before they write to you</Trans>}
          action={<InboxPicker className="max-w-36" />}
        />
        <nav aria-label={t`Help centers`} className="flex min-h-0 flex-1 flex-col overflow-y-auto px-2 pb-4" ref={list}>
          {centers.isPending || inboxQuery.isPending ? (
            <div className="flex flex-col gap-2 p-2">
              <Skeleton className="h-14 w-full" />
              <Skeleton className="h-14 w-full" />
            </div>
          ) : centers.error ? (
            <ErrorLine error={centers.error} className="p-3" />
          ) : groups.length === 0 ? (
            <p className="px-4 py-8 text-center text-body text-faint" data-testid="help-empty">
              <Trans>No inbox to show a help center for.</Trans>
            </p>
          ) : (
            groups.map(({ inbox, center }) => (
              <section key={inbox.id} className="flex flex-col" data-testid="help-inbox">
                <ColumnHeading>
                  <span className="flex items-center gap-1.5">
                    <Dot color={inbox.branding.color} className="size-[7px] rounded-[2px]" />
                    {inbox.name}
                  </span>
                </ColumnHeading>
                {center ? (
                  <ul className="flex flex-col">
                    <CenterRow c={center} current={center.id === currentId} cursor={index++ === cursor} />
                  </ul>
                ) : canManage ? (
                  <div className="px-2.5 py-2">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => setCreating(inbox)}
                      title={t`Create help center (${newKey})`}
                      data-testid="create-help-center-open"
                    >
                      <PlusIcon />
                      <Trans>Create help center</Trans>
                    </Button>
                  </div>
                ) : (
                  <p className="px-2.5 py-2 text-small text-faint" data-testid="help-none">
                    <Trans>No help center yet. Owners and admins create it.</Trans>
                  </p>
                )}
              </section>
            ))
          )}
        </nav>
      </section>
      {creating && (
        <CreateCenterDialog
          inboxId={creating.id}
          inboxName={creating.name}
          onClose={() => setCreating(null)}
          onCreated={(c) => {
            setCreating(null)
            navigate(centerLink(c.id, "settings"))
          }}
        />
      )}
    </>
  )
}
