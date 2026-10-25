import { Trans, useLingui } from "@lingui/react/macro"
import {
  ArrowBigUpIcon,
  ArrowUpRightIcon,
  BellOffIcon,
  CheckCheckIcon,
  CircleCheckIcon,
  CircleDotIcon,
  EyeOffIcon,
  HourglassIcon,
  LayoutGridIcon,
  LockIcon,
  LogOutIcon,
  MessageSquareIcon,
  ReplyIcon,
} from "lucide-react"
import { useState } from "react"
import { Link, useSearchParams } from "react-router"

import { ErrorLine, toast } from "@/components/common"
import { InitialsAvatar, useAuthorName } from "@/components/common/DiscussionAuthor"
import { OverflowTabs } from "@/components/common/OverflowTabs"
import { formatRelative, useErrorText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { AccountLayout, Unreachable, usePersonLocale } from "@/features/account/AccountLayout"
import {
  threadOnPage,
  useMarkNotificationsRead,
  useReaderAccount,
  useReaderDiscussions,
  useReaderNotifications,
  useUnfollow,
  useUpdateReaderAccount,
} from "@/features/account/queries"
import { DeleteAccountButton } from "@/features/auth/DeleteAccountButton"
import { AuthLayout } from "@/features/auth/AuthLayout"
import { SignInForm } from "@/features/auth/SignInForm"
import { pageLabel } from "@/features/docs/PageContext"
import { Card, EmptyRow, Field, Section, ToggleRow } from "@/features/settings/ui"
import type { ReaderAccount, ReaderDiscussion, ReaderNotification } from "@/lib/api"
import { useMe, useSignOut } from "@/lib/session"
import { cn } from "@/lib/utils"

type Tab = "notifications" | "discussions" | "following" | "settings"

const TABS: Tab[] = ["notifications", "discussions", "following", "settings"]

function LoadMore({
  list,
}: {
  list: { hasNextPage: boolean; isFetchingNextPage: boolean; fetchNextPage: () => unknown }
}) {
  if (!list.hasNextPage) return null
  return (
    <div className="flex justify-center border-t p-2">
      <Button variant="ghost" size="sm" onClick={() => void list.fetchNextPage()} disabled={list.isFetchingNextPage}>
        <Trans>Load more</Trans>
      </Button>
    </div>
  )
}

function NotificationRow({ n, onOpen }: { n: ReaderNotification; onOpen: () => void }) {
  const { i18n } = useLingui()
  const authorName = useAuthorName()
  const who = n.author ? authorName(n.author) : ""
  const title = n.title
  const unread = !n.read_at
  return (
    <li>
      <a
        href={threadOnPage(n.page, n.discussion_id)}
        target="_blank"
        rel="noreferrer"
        onClick={onOpen}
        className="flex items-start gap-3 px-4 py-3.5 transition-colors hover:bg-muted"
        data-testid="account-notification"
        data-unread={unread || undefined}
      >
        <span
          className={cn(
            "mt-0.5 grid size-8 shrink-0 place-items-center rounded-full",
            n.kind === "answer" ? "bg-success/10 text-success" : "bg-muted text-muted-foreground"
          )}
        >
          {n.kind === "answer" ? <CircleCheckIcon className="size-4" /> : <ReplyIcon className="size-4" />}
        </span>
        <span className="flex min-w-0 flex-1 flex-col gap-0.5">
          <span className={cn("text-body", unread ? "font-medium" : "text-muted-foreground")}>
            {n.kind === "answer" ? (
              <Trans>
                An answer was accepted in <span className="break-words">{title}</span>
              </Trans>
            ) : who ? (
              <Trans>
                {who} replied in <span className="break-words">{title}</span>
              </Trans>
            ) : (
              <Trans>
                New reply in <span className="break-words">{title}</span>
              </Trans>
            )}
          </span>
          {n.excerpt && <span className="line-clamp-2 text-body text-muted-foreground">{n.excerpt}</span>}
          <span className="truncate text-caption text-faint">
            {n.inbox_name} · {formatRelative(n.created_at, i18n.locale)}
          </span>
        </span>
        {unread && <span className="mt-2 size-2 shrink-0 rounded-full bg-brand" aria-hidden />}
      </a>
    </li>
  )
}

function Notifications() {
  const { t } = useLingui()
  const [unreadOnly, setUnreadOnly] = useState(true)
  const list = useReaderNotifications(unreadOnly)
  const read = useMarkNotificationsRead()
  const items = list.data?.pages.flatMap((p) => p.items) ?? []
  const unread = list.data?.pages[0]?.unread ?? 0
  return (
    <Section
      title={<Trans>Replies and answers in the discussions you follow</Trans>}
      action={
        <span className="flex items-center gap-3">
          <label className="flex items-center gap-2 text-small text-muted-foreground">
            <Switch checked={unreadOnly} onCheckedChange={setUnreadOnly} aria-label={t`Unread only`} />
            <Trans>Unread only</Trans>
          </label>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => read.mutate(undefined, { onSuccess: () => toast(t`All marked as read`) })}
            disabled={unread === 0 || read.isPending}
            data-testid="account-read-all"
          >
            <CheckCheckIcon />
            <Trans>Mark all as read</Trans>
          </Button>
        </span>
      }
    >
      <Card flush data-testid="account-notifications">
        {list.isPending ? (
          <Skeleton className="m-4 h-12" />
        ) : list.error ? (
          <ErrorLine error={list.error} className="p-4" />
        ) : items.length === 0 ? (
          <EmptyRow>
            {unreadOnly ? <Trans>Nothing unread.</Trans> : <Trans>No notifications in the last 90 days.</Trans>}
          </EmptyRow>
        ) : (
          <ul className="divide-y">
            {items.map((n) => (
              <NotificationRow key={n.id} n={n} onOpen={() => !n.read_at && read.mutate([n.id])} />
            ))}
          </ul>
        )}
        <LoadMore list={list} />
      </Card>
    </Section>
  )
}

function StateIcon({ d }: { d: ReaderDiscussion }) {
  const { t } = useLingui()
  const [Icon, tone, label] =
    d.state === "hidden"
      ? [EyeOffIcon, "text-faint", t`Hidden by the team`]
      : d.state === "pending"
        ? [HourglassIcon, "text-warning", t`Waiting for approval`]
        : d.answered
          ? [CircleCheckIcon, "text-success", t`Answered`]
          : d.locked
            ? [LockIcon, "text-faint", t`Locked`]
            : [CircleDotIcon, "text-brand", t`Open`]
  return (
    <span title={label} className={cn("mt-0.5 shrink-0", tone)}>
      <Icon className="size-4" />
      <span className="sr-only">{label}</span>
    </span>
  )
}

function DiscussionRow({ d, onUnfollow }: { d: ReaderDiscussion; onUnfollow?: () => void }) {
  const { t, i18n } = useLingui()
  const fmt = new Intl.NumberFormat(i18n.locale)
  const [votes, replies] = [d.votes, d.reply_count].map((n) => fmt.format(n))
  return (
    <li className="flex items-center gap-1 pe-2" data-testid="account-discussion">
      <a
        href={threadOnPage(d.page, d.id)}
        target="_blank"
        rel="noreferrer"
        className="flex min-w-0 flex-1 items-start gap-3 py-3 ps-4 pe-2 transition-colors hover:bg-muted"
      >
        <StateIcon d={d} />
        <span className="flex min-w-0 flex-1 flex-col gap-0.5">
          <span className="flex items-center gap-2">
            <span className="min-w-0 flex-1 truncate text-body font-medium">{d.title}</span>
            <ArrowUpRightIcon className="size-3.5 shrink-0 text-faint" />
          </span>
          <span className="truncate text-caption text-faint">
            {d.inbox_name} · <span dir="ltr">{pageLabel(d.page, d.page_title)}</span>
          </span>
          <span className="flex flex-wrap items-center gap-x-2.5 gap-y-0.5 text-caption text-faint">
            <span>{d.started ? <Trans>You started it</Trans> : <Trans>You replied</Trans>}</span>
            {d.state === "pending" && (
              <span className="font-medium text-warning">
                <Trans>Waiting for approval</Trans>
              </span>
            )}
            {d.state === "hidden" && (
              <span className="font-medium">
                <Trans>Hidden by the team</Trans>
              </span>
            )}
            <span className="inline-flex items-center gap-1 tabular-nums" title={t`${votes} votes`}>
              <ArrowBigUpIcon className="size-3.5" />
              {votes}
            </span>
            <span className="inline-flex items-center gap-1 tabular-nums" title={t`${replies} replies`}>
              <MessageSquareIcon className="size-3.5" />
              {replies}
            </span>
            <span>{formatRelative(d.last_activity_at, i18n.locale)}</span>
          </span>
        </span>
      </a>
      {onUnfollow && (
        <Button variant="ghost" size="sm" onClick={onUnfollow} data-testid="account-unfollow">
          <BellOffIcon />
          <span className="phone:sr-only">
            <Trans>Unfollow</Trans>
          </span>
        </Button>
      )}
    </li>
  )
}

function Discussions({ kind }: { kind: "discussions" | "following" }) {
  const { t } = useLingui()
  const errorText = useErrorText()
  const list = useReaderDiscussions(kind)
  const unfollow = useUnfollow()
  const items = list.data?.pages.flatMap((p) => p.items) ?? []
  return (
    <Section
      title={
        kind === "discussions" ? (
          <Trans>Discussions you started or replied to</Trans>
        ) : (
          <Trans>You get a notification when someone replies in these</Trans>
        )
      }
    >
      <Card flush data-testid={`account-${kind}`}>
        {list.isPending ? (
          <Skeleton className="m-4 h-12" />
        ) : list.error ? (
          <ErrorLine error={list.error} className="p-4" />
        ) : items.length === 0 ? (
          <EmptyRow>
            {kind === "discussions" ? (
              <Trans>You have not written in any discussion yet.</Trans>
            ) : (
              <Trans>You follow no discussions. You follow the ones you start or reply to.</Trans>
            )}
          </EmptyRow>
        ) : (
          <ul className="divide-y">
            {items.map((d) => (
              <DiscussionRow
                key={d.id}
                d={d}
                onUnfollow={
                  kind === "following"
                    ? () =>
                        unfollow.mutate(d.id, {
                          onSuccess: () => toast(t`Unfollowed`),
                          onError: (e) => toast(errorText(e)),
                        })
                    : undefined
                }
              />
            ))}
          </ul>
        )}
        <LoadMore list={list} />
      </Card>
    </Section>
  )
}

function Settings({ account }: { account: ReaderAccount }) {
  const { t } = useLingui()
  const update = useUpdateReaderAccount()
  const [name, setName] = useState(account.name)
  const changed = name.trim() !== account.name
  return (
    <div className="flex flex-col gap-8">
      <Section title={<Trans>Profile</Trans>}>
        <Card>
          <form
            className="flex flex-col gap-4"
            onSubmit={(e) => {
              e.preventDefault()
              update.mutate({ name: name.trim() }, { onSuccess: () => toast(t`Saved`) })
            }}
          >
            <Field
              label={<Trans>Name</Trans>}
              htmlFor="account-name"
              hint={<Trans>Shown on your posts in every documentation site. Without one you show as “Reader”.</Trans>}
            >
              <div className="flex flex-wrap gap-2">
                <Input
                  id="account-name"
                  value={name}
                  maxLength={200}
                  autoComplete="name"
                  onChange={(e) => setName(e.target.value)}
                  className="min-w-0 flex-1"
                  data-testid="account-name"
                />
                <Button type="submit" disabled={!changed || update.isPending} data-testid="account-name-save">
                  <Trans>Save</Trans>
                </Button>
              </div>
            </Field>
            <Field label={<Trans>Sign-in address</Trans>}>
              <p className="text-body break-words text-muted-foreground">{account.email}</p>
            </Field>
            <ErrorLine error={update.error} />
          </form>
        </Card>
      </Section>
      <Section title={<Trans>E-mail</Trans>}>
        <Card>
          <ToggleRow
            title={<Trans>E-mail me about the discussions I follow</Trans>}
            hint={
              <Trans>
                At most one e-mail per discussion every few minutes, with the new replies and answers. Notifications
                here stay on.
              </Trans>
            }
          >
            <Switch
              checked={account.email_notifications}
              onCheckedChange={(on) => update.mutate({ email_notifications: on })}
              aria-label={t`E-mail me about the discussions I follow`}
              data-testid="account-email-notifications"
            />
          </ToggleRow>
        </Card>
      </Section>
      <Section title={<Trans>Delete your account</Trans>}>
        <Card tone="danger">
          <p className="text-body text-muted-foreground">
            {account.member ? (
              <Trans>
                This also removes you from every workspace you belong to. Your posts stay, shown as from a deleted
                reader.
              </Trans>
            ) : (
              <Trans>Your posts stay, shown as from a deleted reader. Your follows, votes and notifications go.</Trans>
            )}
          </p>
          <div>
            <DeleteAccountButton
              email={account.email}
              redirect="/account"
              description={
                account.member ? undefined : (
                  <Trans>
                    Your posts stay without your name, shown as from a deleted reader. Your sign-ins, passkeys, follows,
                    votes and notifications go. This cannot be undone.
                  </Trans>
                )
              }
            />
          </div>
        </Card>
      </Section>
    </div>
  )
}

function AccountHome() {
  const { t } = useLingui()
  const signOut = useSignOut()
  const account = useReaderAccount()
  const unread = useReaderNotifications(true).data?.pages[0]?.unread ?? 0
  const [params, setParams] = useSearchParams()
  const raw = params.get("tab")
  const tab: Tab = TABS.includes(raw as Tab) ? (raw as Tab) : "notifications"
  const labels: Record<Tab, string> = {
    notifications: t`Notifications`,
    discussions: t`My discussions`,
    following: t`Following`,
    settings: t`Settings`,
  }
  const a = account.data
  return (
    <AccountLayout>
      {account.isPending ? (
        <Skeleton className="h-20 rounded-xl" />
      ) : !a ? (
        <ErrorLine error={account.error} />
      ) : (
        <div className="flex flex-col gap-7" data-testid="account">
          <header className="flex flex-wrap items-center gap-4">
            <InitialsAvatar initials={a.initials} className="size-12 text-title" />
            <div className="flex min-w-0 flex-1 flex-col">
              <h1 className="truncate text-page">{a.name || <Trans>Your account</Trans>}</h1>
              <p className="truncate text-body text-muted-foreground">{a.email}</p>
            </div>
            <div className="flex flex-wrap gap-2">
              {a.member && (
                <Button variant="outline" render={<Link to="/" />} data-testid="account-team-panel">
                  <LayoutGridIcon />
                  <Trans>Team panel</Trans>
                </Button>
              )}
              <Button variant="ghost" onClick={() => void signOut()} data-testid="account-sign-out">
                <LogOutIcon />
                <Trans>Sign out</Trans>
              </Button>
            </div>
          </header>
          <OverflowTabs
            label={t`Account`}
            current={tab}
            testId="account-tabs"
            className="-mx-4 -mt-2 border-b px-4 sm:mx-0 sm:px-1"
            items={TABS.map((k) => ({
              key: k,
              label: (
                <>
                  {labels[k]}
                  {k === "notifications" && unread > 0 && (
                    <span className="min-w-5 rounded-full bg-brand px-1.5 text-center text-caption font-medium text-white tabular-nums">
                      {unread}
                    </span>
                  )}
                </>
              ),
              testId: `account-tab-${k}`,
              onSelect: () => setParams(k === "notifications" ? {} : { tab: k }, { replace: true }),
            }))}
          />
          {tab === "notifications" && <Notifications />}
          {tab === "discussions" && <Discussions kind="discussions" />}
          {tab === "following" && <Discussions kind="following" />}
          {tab === "settings" && <Settings key={a.person_id} account={a} />}
        </div>
      )}
    </AccountLayout>
  )
}

export function AccountPage() {
  const me = useMe()
  usePersonLocale()
  if (me.isPending) return <div className="min-h-svh bg-background" />
  if (me.error && !me.data) return <Unreachable onRetry={() => void me.refetch()} />
  if (!me.data)
    return (
      <AuthLayout>
        <SignInForm
          reader
          title={<Trans>Your Yuva account</Trans>}
          lead={
            <Trans>
              Sign in or create an account with your e-mail address to follow the discussions you write in on
              documentation sites. We e-mail you a one-time code; no password.
            </Trans>
          }
        />
      </AuthLayout>
    )
  return <AccountHome />
}
