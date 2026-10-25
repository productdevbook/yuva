import { Trans, useLingui } from "@lingui/react/macro"
import { PlusIcon } from "lucide-react"
import { Link } from "react-router"

import { InboxMark } from "@/components/common"
import { keyLabel, ROADMAP_KEYS } from "@/components/common/ShortcutSheet"
import { Button } from "@/components/ui/button"
import { look } from "@/features/inbox/PageFrame"
import { boardLink } from "@/features/roadmap/queries"
import { Pill } from "@/features/status/PageFrame"
import type { Inbox, RoadmapBoard } from "@/lib/api"
import { cn } from "@/lib/utils"

export const openIdeas = (b: RoadmapBoard) =>
  b.counts.ideas + b.counts.planned + b.counts.in_progress + b.counts.shipped

function Count({ n }: { n: number }) {
  return <span className="font-normal opacity-65 tabular-nums">{n}</span>
}

export function BoardBar({
  boards,
  inboxes,
  selected,
  all,
  settings,
  missing,
  canManage,
  busy,
  onCreate,
  onTogglePublic,
}: {
  boards: RoadmapBoard[]
  inboxes: Map<string, Inbox>
  selected?: RoadmapBoard
  all: boolean
  settings: boolean
  missing: Inbox[]
  canManage: boolean
  busy: boolean
  onCreate: (i: Inbox) => void
  onTogglePublic: (b: RoadmapBoard) => void
}) {
  const { t, i18n } = useLingui()
  const fmt = new Intl.NumberFormat(i18n.locale)
  const publicKey = keyLabel(ROADMAP_KEYS.publicBoard).join("")
  const newKey = keyLabel(ROADMAP_KEYS.newBoard).join("")
  const chips = boards.length > 1 || missing.length > 0
  if (!chips && !selected) return null
  const mark = (b: RoadmapBoard) => {
    const inbox = inboxes.get(b.inbox_id)
    return <InboxMark name={inbox?.name ?? b.name} color={inbox?.branding.color} className="size-4 text-[9px]" />
  }
  return (
    <div
      className="flex shrink-0 flex-wrap items-center gap-x-3 gap-y-2 border-b px-6 py-2.5 @max-3xl:px-4"
      data-testid="board-bar"
    >
      {chips ? (
        <nav
          aria-label={t`Roadmap boards`}
          className="-my-1 flex min-w-0 flex-1 basis-64 items-center gap-2 overflow-x-auto py-1 [scrollbar-width:none]"
          data-testid="board-chips"
        >
          {!settings && boards.length > 1 && (
            <Link
              to="/board"
              aria-current={all ? "page" : undefined}
              className={all ? look.listChipOn : look.listChip}
              data-testid="board-chip-all"
            >
              <Trans>All boards</Trans>
              <Count n={boards.reduce((n, b) => n + openIdeas(b), 0)} />
            </Link>
          )}
          {boards.map((b) => {
            const on = !all && selected?.id === b.id
            return (
              <Link
                key={b.id}
                to={boardLink(b.id, settings ? "settings" : undefined)}
                aria-current={on ? "page" : undefined}
                className={on ? look.listChipOn : look.listChip}
                data-testid="board-chip"
              >
                {mark(b)}
                <span className="max-w-40 truncate">{b.name}</span>
                <Count n={openIdeas(b)} />
              </Link>
            )
          })}
          {canManage &&
            missing.map((i) => {
              const name = i.name
              return (
                <button
                  key={i.id}
                  type="button"
                  className={cn(look.listChip, "border-dashed bg-transparent")}
                  onClick={() => onCreate(i)}
                  title={t`Create board (${newKey})`}
                  data-testid="create-board-open"
                >
                  <PlusIcon className="size-3.5" />
                  <span className="whitespace-nowrap">
                    <Trans>Create board for {name}</Trans>
                  </span>
                </button>
              )
            })}
        </nav>
      ) : (
        selected && (
          <span className="flex min-w-0 flex-1 basis-64 items-center gap-2">
            {mark(selected)}
            <span className="truncate font-semibold" data-testid="board-name">
              {selected.name}
            </span>
            <span className="text-caption text-faint tabular-nums">{fmt.format(openIdeas(selected))}</span>
          </span>
        )
      )}
      {selected && !all && (
        <div className="flex min-w-0 items-center gap-2.5 @max-3xl:flex-wrap" data-testid="board-state">
          <Pill tone={selected.public ? "success" : "muted"} data-testid="board-visibility">
            {selected.public ? <Trans>Public</Trans> : <Trans>Private</Trans>}
          </Pill>
          {selected.public && (
            <a
              href={selected.url}
              target="_blank"
              rel="noreferrer noopener"
              className={cn(look.faint, "max-w-64 truncate hover:text-foreground hover:underline @max-3xl:max-w-40")}
              dir="ltr"
              data-testid="board-url"
            >
              {selected.url.replace(/^https?:\/\//, "")}
            </a>
          )}
          {canManage && (
            <Button
              variant="outline"
              className={look.btnSm}
              disabled={busy}
              onClick={() => onTogglePublic(selected)}
              title={selected.public ? t`Make private (${publicKey})` : t`Make public (${publicKey})`}
              data-testid="board-toggle-public"
            >
              {selected.public ? <Trans>Make private</Trans> : <Trans>Make public</Trans>}
            </Button>
          )}
        </div>
      )}
    </div>
  )
}
