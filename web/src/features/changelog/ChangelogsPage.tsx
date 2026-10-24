import "@/features/changelog/i18n"
import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowUpRightIcon, PlusIcon } from "lucide-react"
import { useState } from "react"
import { useNavigate } from "react-router"

import { InboxMark } from "@/components/common"
import { PaneBack } from "@/components/common/Column"
import { CHANGELOG_KEYS, keyLabel } from "@/components/common/ShortcutSheet"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { CreateChangelogDialog } from "@/features/changelog/CreateChangelogDialog"
import { PreviewDialog } from "@/features/changelog/PreviewDialog"
import { changelogLink, changelogOfInbox, useChangelogs, useUpdateChangelog } from "@/features/changelog/queries"
import { ErrorLine } from "@/features/changelog/text"
import { useLocaleName } from "@/features/help/text"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { look, PageTop } from "@/features/inbox/PageFrame"
import { Pill } from "@/features/status/PageFrame"
import { useHotkeys } from "@/hooks/use-hotkeys"
import type { Changelog, Inbox } from "@/lib/api"
import { useSession } from "@/lib/session"
import { isSupport, useInboxes } from "@/lib/workspace"

function ChangelogCard({ changelog, inbox }: { changelog: Changelog; inbox: Inbox }) {
  const navigate = useNavigate()
  const { canManage } = useSession()
  const localeName = useLocaleName()
  const patch = useUpdateChangelog(changelog.id)
  const [preview, setPreview] = useState(false)
  const { entries: total, published, scheduled } = changelog.counts
  return (
    <section className={look.card} data-testid="changelog-card">
      <div className="flex flex-wrap items-center gap-3 px-4 py-4">
        <InboxMark name={inbox.name} color={inbox.branding.color} className="size-8 text-body" />
        <div className="flex min-w-48 flex-1 flex-col leading-[1.35]">
          <span className="flex items-center gap-2">
            <span className="truncate text-[15px] font-semibold">{changelog.name}</span>
            <Pill tone={changelog.published ? "success" : "muted"} data-testid="changelog-card-state">
              {changelog.published ? <Trans>Published</Trans> : <Trans>Unpublished</Trans>}
            </Pill>
          </span>
          <span className="truncate text-small text-faint">
            {changelog.url.replace(/^https?:\/\//, "")}
            {" · "}
            <Trans>
              {published} of {total} entries published
            </Trans>
            {scheduled > 0 && (
              <>
                {" · "}
                <Trans>Scheduled: {scheduled}</Trans>
              </>
            )}
          </span>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" className={look.btnSm} onClick={() => navigate(changelogLink(changelog.id))}>
            <Trans>Entries</Trans>
          </Button>
          <Button variant="outline" className={look.btnSm} onClick={() => setPreview(true)}>
            <Trans>Preview</Trans>
          </Button>
          {changelog.published && (
            <Button
              variant="outline"
              className={look.btnSm}
              render={<a href={changelog.url} target="_blank" rel="noreferrer noopener" />}
            >
              <Trans>View changelog</Trans>
              <ArrowUpRightIcon className="rtl:-scale-x-100" />
            </Button>
          )}
          {canManage && (
            <>
              <Button
                variant={changelog.published ? "outline" : "default"}
                className={changelog.published ? look.btnSm : look.btnInkSm}
                disabled={patch.isPending}
                onClick={() => patch.mutate({ published: !changelog.published })}
                data-testid="changelog-card-publish"
              >
                {changelog.published ? <Trans>Unpublish</Trans> : <Trans>Publish</Trans>}
              </Button>
              <Button
                variant="outline"
                className={look.btnSm}
                onClick={() => navigate(changelogLink(changelog.id, "settings"))}
              >
                <Trans>Settings</Trans>
              </Button>
            </>
          )}
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-1.5 px-4 pb-3.5">
        <span className={look.faint}>
          <Trans>Languages</Trans>
        </span>
        {changelog.locales.map((l) => (
          <span
            key={l}
            className="rounded-[4px] border bg-surface px-1.5 text-[11px] leading-4 text-muted-foreground"
            title={localeName(l)}
          >
            {localeName(l)}
          </span>
        ))}
      </div>
      <ErrorLine error={patch.error} className="px-4 pb-3" />
      {preview && <PreviewDialog changelog={changelog} onClose={() => setPreview(false)} />}
    </section>
  )
}

export function ChangelogsPage() {
  const navigate = useNavigate()
  const { canManage } = useSession()
  const [inboxId] = useInboxFilter()
  const inboxQuery = useInboxes()
  const changelogs = useChangelogs()
  const [creating, setCreating] = useState<Inbox | null>(null)
  const inboxes = (inboxQuery.data ?? []).filter((i) => isSupport(i) && (!inboxId || i.id === inboxId))
  const missing = canManage ? inboxes.find((i) => !changelogOfInbox(changelogs.data, i.id)) : undefined
  const createKey = keyLabel(CHANGELOG_KEYS.create).join("")
  useHotkeys(missing ? { [CHANGELOG_KEYS.create]: () => setCreating(missing) } : {}, !creating)
  const { t } = useLingui()
  return (
    <main className="flex min-h-0 min-w-0 flex-1 flex-col bg-background" data-testid="changelogs-page">
      <PageTop
        title={<Trans>Changelogs</Trans>}
        sub={<Trans>Each product has one changelog. Owners and admins create and publish them.</Trans>}
      />
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="flex max-w-[980px] flex-col gap-4 p-6 pb-16 phone:p-4">
          <PaneBack to="/updates" label={t`What’s new`} always />
          <ErrorLine error={changelogs.error} />
          {changelogs.isPending || inboxQuery.isPending ? (
            <>
              <Skeleton className="h-24 w-full rounded-xl" />
              <Skeleton className="h-24 w-full rounded-xl" />
            </>
          ) : (
            inboxes.map((inbox) => {
              const changelog = changelogOfInbox(changelogs.data, inbox.id)
              return changelog ? (
                <ChangelogCard key={inbox.id} changelog={changelog} inbox={inbox} />
              ) : (
                <section
                  key={inbox.id}
                  className="flex flex-wrap items-center gap-3 rounded-xl border border-dashed px-4 py-4"
                  data-testid="changelog-card-missing"
                >
                  <InboxMark name={inbox.name} color={inbox.branding.color} className="size-8 text-body" />
                  <div className="flex min-w-48 flex-1 flex-col leading-[1.35]">
                    <span className="truncate text-[15px] font-semibold">{inbox.name}</span>
                    {!canManage && (
                      <span className="text-small text-faint">
                        <Trans>No changelog yet. Owners and admins create it.</Trans>
                      </span>
                    )}
                  </div>
                  {canManage && (
                    <Button
                      variant="outline"
                      className={look.btnSm}
                      onClick={() => setCreating(inbox)}
                      title={t`Create changelog (${createKey})`}
                      data-testid="create-changelog-open"
                    >
                      <PlusIcon />
                      <Trans>Create changelog</Trans>
                    </Button>
                  )}
                </section>
              )
            })
          )}
        </div>
      </div>
      {creating && (
        <CreateChangelogDialog
          inboxId={creating.id}
          inboxName={creating.name}
          onClose={() => setCreating(null)}
          onCreated={(c) => {
            setCreating(null)
            navigate(changelogLink(c.id, "settings"))
          }}
        />
      )}
    </main>
  )
}
