import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowUpRightIcon, PlusIcon } from "lucide-react"
import { Link } from "react-router"

import { useInboxFilter } from "@/features/inbox/inboxFilter"

import { InboxMark, useConfirm } from "@/components/common"
import { HELP_KEYS, keyLabel } from "@/components/common/ShortcutSheet"
import { Button } from "@/components/ui/button"
import { centerLink, useUpdateHelpCenter } from "@/features/help/queries"
import { look } from "@/features/inbox/PageFrame"
import { Pill } from "@/features/status/PageFrame"
import type { HelpCenter, Inbox } from "@/lib/api"
import { cn } from "@/lib/utils"

export type CenterGroup = { inbox: Inbox; center?: HelpCenter }

export function DocsNote() {
  return (
    <p className={look.faint} data-testid="help-docs-note">
      <Trans>
        Written here, shown on pages Yuva hosts, searched from the chat widget and the reply box. Documentation pages on
        your own site live under{" "}
        <Link to="/docs" className="text-brand hover:underline">
          Docs
        </Link>
        .
      </Trans>
    </p>
  )
}

export function ProductChips({
  groups,
  currentInboxId,
  canManage,
  onCreate,
}: {
  groups: CenterGroup[]
  currentInboxId?: string
  canManage: boolean
  onCreate: (i: Inbox) => void
}) {
  const { t } = useLingui()
  const [filter, setFilter] = useInboxFilter()
  if (groups.length < 2) return null
  return (
    <nav
      aria-label={t`Help center`}
      className="-my-1 flex min-w-0 items-center gap-2 overflow-x-auto py-1 [scrollbar-width:none] phone:flex-1"
      data-testid="help-center-picker"
    >
      {groups.map(({ inbox, center }) => {
        const on = inbox.id === currentInboxId
        const dot = (
          <span
            className={`size-1.5 shrink-0 rounded-full ${center?.published ? "bg-success" : "bg-faint/50"}`}
            aria-hidden
          />
        )
        const mark = <InboxMark name={inbox.name} color={inbox.branding.color} className="size-4 text-[9px]" />
        if (center) {
          return (
            <Link
              key={inbox.id}
              to={centerLink(center.id)}
              aria-current={on ? "page" : undefined}
              className={on ? look.listChipOn : look.listChip}
              onClick={() => filter && setFilter(inbox.id)}
              data-testid="help-row"
            >
              {mark}
              <span className="max-w-40 truncate">{inbox.name}</span>
              {dot}
            </Link>
          )
        }
        return canManage ? (
          <button
            key={inbox.id}
            type="button"
            className={cn(on ? look.listChipOn : look.listChip, !on && "border-dashed bg-transparent")}
            onClick={() => onCreate(inbox)}
            data-testid="create-help-center-open"
          >
            <PlusIcon className="size-3.5" />
            <span className="max-w-40 truncate">{inbox.name}</span>
          </button>
        ) : null
      })}
    </nav>
  )
}

export function CenterBar({
  center,
  groups,
  canManage,
  busy,
  onCreate,
  onTogglePublish,
}: {
  center: HelpCenter
  groups: CenterGroup[]
  canManage: boolean
  busy: boolean
  onCreate: (i: Inbox) => void
  onTogglePublish: () => void
}) {
  const { t } = useLingui()
  const publishKey = keyLabel(HELP_KEYS.publishCenter).join("")
  const openKey = keyLabel(HELP_KEYS.openPage).join("")
  const inbox = groups.find((g) => g.inbox.id === center.inbox_id)?.inbox
  return (
    <div
      className="flex shrink-0 flex-wrap items-center gap-x-3 gap-y-2 border-b bg-surface px-6 py-2.5 phone:px-4"
      data-testid="help-center-bar"
    >
      <span className="flex min-w-0 flex-1 basis-64 items-center gap-2 phone:basis-full">
        {inbox && <InboxMark name={inbox.name} color={inbox.branding.color} className="size-5 text-[10px]" />}
        <span className="flex min-w-0 flex-col leading-[1.3]">
          <span className="flex items-center gap-2">
            <span className="truncate text-reading font-semibold" data-testid="help-center-name">
              {center.name}
            </span>
            <Pill tone={center.published ? "success" : "muted"} data-testid="help-published">
              {center.published ? <Trans>Published</Trans> : <Trans>Unpublished</Trans>}
            </Pill>
          </span>
          <span className={`${look.faint} truncate`} dir="ltr">
            {center.url.replace(/^https?:\/\//, "")}
          </span>
        </span>
      </span>
      <ProductChips groups={groups} currentInboxId={center.inbox_id} canManage={canManage} onCreate={onCreate} />
      <div className="flex shrink-0 items-center gap-2 phone:ms-auto">
        {center.published && (
          <Button
            variant="outline"
            className={look.btnSm}
            render={<a href={center.url} target="_blank" rel="noreferrer noopener" />}
            title={t`View site (${openKey})`}
            data-testid="help-open-public"
          >
            <Trans>View site</Trans>
            <ArrowUpRightIcon className="text-faint" />
          </Button>
        )}
        {canManage &&
          (center.published ? (
            <Button
              variant="outline"
              className={look.btnSm}
              disabled={busy}
              onClick={onTogglePublish}
              title={t`Unpublish (${publishKey})`}
              data-testid="help-publish"
            >
              <Trans>Unpublish</Trans>
            </Button>
          ) : (
            <Button
              className={look.btnInkSm}
              disabled={busy}
              onClick={onTogglePublish}
              title={t`Publish (${publishKey})`}
              data-testid="help-publish"
            >
              <Trans>Publish</Trans>
            </Button>
          ))}
      </div>
    </div>
  )
}

export function usePublishToggle(centerId: string, published: boolean | undefined) {
  const { t } = useLingui()
  const patch = useUpdateHelpCenter(centerId)
  const [confirm, dialog] = useConfirm()
  const toggle = () => {
    if (!published) return patch.mutate({ published: true })
    confirm({
      title: t`Unpublish this help center?`,
      description: t`Visitors can no longer open it and the chat widget stops suggesting its articles. Your articles stay as they are.`,
      confirm: t`Unpublish`,
      run: () => patch.mutate({ published: false }),
    })
  }
  return { toggle, dialog, pending: patch.isPending, error: patch.error }
}
