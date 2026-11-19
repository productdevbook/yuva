import { plural } from "@lingui/core/macro"
import { Trans, useLingui } from "@lingui/react/macro"
import { useState } from "react"
import { Link } from "react-router"

import { PageLayout, toast } from "@/components/common"
import { HOME_KEYS } from "@/components/common/ShortcutSheet"
import { useErrorText } from "@/components/common/text"
import { Alert, AlertAction, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Item, ItemActions, ItemContent, ItemDescription, ItemTitle } from "@/components/ui/item"
import { Progress } from "@/components/ui/progress"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { BlockLink } from "@/features/home/HomeBlock"
import { LaterToday, Products, Recent, useLaterRows, useProductRows, useRecent } from "@/features/home/HomeSide"
import { MyConversations, Today, useLate, useMine, useWaitingRows, WaitingForYou } from "@/features/home/HomeWork"
import { useHome } from "@/features/home/queries"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { useBulkUpdateConversations } from "@/features/inbox/queries"
import { useHubText } from "@/features/setup/SetupHub"
import { HOME_BACK, orderByFocus, startPath, type HubId, type SetupState } from "@/features/setup/setup"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useNow } from "@/hooks/use-now"
import type { Home, HomeSetupItem } from "@/lib/api"
import { useSetAvailability } from "@/lib/availability"
import { useRealtimeStatus } from "@/lib/realtime"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useInboxes, useMembers } from "@/lib/workspace"

const CARD_ITEMS = 4

function hiddenKey(ws: string, member: string) {
  return `home-setup-hidden:${ws}:${member}`
}

function useSetupHidden(): [boolean, (hidden: boolean) => void] {
  const { workspaceId, membership } = useSession()
  const key = hiddenKey(workspaceId, membership.member_id)
  const [hidden, setHidden] = useState(() => {
    try {
      return localStorage.getItem(key) === "1"
    } catch {
      return false
    }
  })
  const set = (next: boolean) => {
    setHidden(next)
    try {
      if (next) localStorage.setItem(key, "1")
      else localStorage.removeItem(key)
    } catch {
      // Storage can be blocked; the card then stays hidden until the page reloads.
    }
  }
  return [hidden, set]
}

function useSteps() {
  const hasInbox = (useInboxes().data ?? []).length > 0
  return (id: HubId) =>
    ({
      chat: hasInbox ? 3 : 4,
      app: hasInbox ? 3 : 4,
      docs: hasInbox ? 3 : 4,
      email: hasInbox ? 4 : 6,
      mail: 6,
      analytics: 3,
      newsletter: 3,
    })[id]
}

function Greeting() {
  const { t, i18n } = useLingui()
  const { me, membership } = useSession()
  const now = useNow(60_000)
  const first = (me.person.name || me.person.email).split(/[\s@]/)[0]
  const hour = new Date(now).getHours()
  const greeting =
    hour < 12 ? t`Good morning, ${first}.` : hour < 18 ? t`Good afternoon, ${first}.` : t`Good evening, ${first}.`
  const date = new Intl.DateTimeFormat(i18n.locale, { weekday: "long", day: "numeric", month: "long" }).format(now)
  return (
    <div className="min-w-0 flex-[1_1_280px]">
      <h1 className="m-0 text-[22px] leading-7 font-semibold tracking-[-0.01em]">{greeting}</h1>
      <p className="m-0 mt-0.5 truncate text-small text-muted-foreground" data-testid="home-date">
        {`${date} · ${membership.workspace.name}`}
      </p>
    </div>
  )
}

function Availability() {
  const { t, i18n } = useLingui()
  const { me } = useSession()
  const set = useSetAvailability()
  const live = useRealtimeStatus() === "live"
  const members = useMembers().data ?? []
  const fmt = new Intl.NumberFormat(i18n.locale)
  const online = fmt.format(members.filter((m) => m.online).length)
  const total = fmt.format(members.length)
  const available = me.person.availability !== "away"
  return (
    <div
      className="flex h-9 shrink-0 items-center gap-2.5 rounded-lg border bg-card ps-3 pe-3 text-small"
      data-testid="home-availability"
    >
      <span
        aria-hidden
        className={cn("size-2 rounded-full", available ? (live ? "bg-success" : "bg-faint") : "bg-warn")}
      />
      <label htmlFor="home-available" className="font-medium">
        {available ? t`Available` : t`Away`}
      </label>
      <Switch
        id="home-available"
        checked={available}
        disabled={set.isPending}
        onCheckedChange={(on) => set.mutate(on ? "auto" : "away")}
        data-testid="home-availability-switch"
      />
      {members.length > 1 && (
        <>
          <span aria-hidden className="h-4 w-px bg-border" />
          <Link to="/team" className="text-muted-foreground hover:text-foreground hover:underline">
            <Trans>
              {online} of {total} online
            </Trans>
          </Link>
        </>
      )}
    </div>
  )
}

function cardItems(h: Home, focus: Parameters<typeof orderByFocus>[1]): HomeSetupItem[] {
  return orderByFocus(h.setup?.items ?? [], focus).slice(0, CARD_ITEMS)
}

function SetupCard({ items, onHide }: { items: HomeSetupItem[]; onHide: () => void }) {
  const { t, i18n } = useLingui()
  const { membership } = useSession()
  const text = useHubText()
  const steps = useSteps()
  const name = membership.workspace.name
  const done = items.filter((i) => i.done).length
  const fmt = new Intl.NumberFormat(i18n.locale)
  const doneText = fmt.format(done)
  const total = fmt.format(items.length)
  const back = { back: HOME_BACK } satisfies SetupState
  return (
    <section aria-labelledby="home-setup-title" data-testid="home-setup-card">
      <Card size="sm" className="gap-0 py-0">
        <div className="flex min-h-11 flex-wrap items-center gap-x-3 gap-y-1 px-4 py-2">
          <h2 id="home-setup-title" className="m-0 text-[15px] leading-5 font-semibold">
            {done === 0 ? t`Set up ${name}` : t`Finish setting up ${name}`}
          </h2>
          <span className="text-small text-muted-foreground tabular-nums" data-testid="home-setup-count">
            {t`${doneText} of ${total} done`}
          </span>
          <span className="flex-1" />
          <BlockLink to="/setup" state={back}>
            <Trans>All steps</Trans>
          </BlockLink>
          <Button
            variant="link"
            size="sm"
            className="h-auto p-0 text-small font-medium text-foreground underline hover:text-muted-foreground"
            onClick={onHide}
            data-testid="home-setup-hide"
          >
            <Trans>Hide</Trans>
          </Button>
        </div>
        <Progress
          value={items.length ? Math.round((done / items.length) * 100) : 0}
          aria-label={t`Setup`}
          className="[&_[data-slot=progress-indicator]]:bg-success [&_[data-slot=progress-track]]:h-1 [&_[data-slot=progress-track]]:rounded-none"
          data-testid="home-setup-progress"
        />
        <div role="list" className="flex flex-col">
          {items
            .filter((i) => !i.done)
            .map((item) => {
              const n = steps(item.id)
              return (
                <Item
                  key={item.id}
                  role="listitem"
                  size="sm"
                  className="flex-nowrap rounded-none border-0 border-b border-line-soft px-4 last:border-b-0"
                  data-testid={`home-setup-${item.id}`}
                >
                  <ItemContent className="min-w-0 gap-0.5">
                    <ItemTitle className="block w-full truncate">{text.title[item.id]}</ItemTitle>
                    <ItemDescription className="truncate">{text.hint[item.id]}</ItemDescription>
                  </ItemContent>
                  <ItemActions className="shrink-0 gap-3">
                    <span className="text-small text-muted-foreground phone:hidden">
                      {plural(n, { one: "# step", other: "# steps" })}
                    </span>
                    <Button size="sm" render={<Link to={startPath(item.id)} state={back} />}>
                      <Trans>Set up</Trans>
                    </Button>
                  </ItemActions>
                </Item>
              )
            })}
        </div>
      </Card>
    </section>
  )
}

function HomeSkeleton() {
  return (
    <div aria-hidden className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_340px]" data-testid="home-loading">
      <div className="flex flex-col gap-4">
        <Skeleton className="h-[260px] rounded-xl" />
        <Skeleton className="h-[200px] rounded-xl" />
      </div>
      <Skeleton className="h-[320px] rounded-xl" />
    </div>
  )
}

function useRowKeys() {
  const { t } = useLingui()
  const { membership } = useSession()
  const errorText = useErrorText()
  const bulk = useBulkUpdateConversations()
  const rows = () => Array.from(document.querySelectorAll<HTMLAnchorElement>("[data-testid=home] a[data-home-row]"))
  const move = (step: number) => {
    const all = rows()
    if (all.length === 0) return
    const i = all.indexOf(document.activeElement as HTMLAnchorElement)
    const next = i < 0 ? (step > 0 ? 0 : all.length - 1) : Math.min(all.length - 1, Math.max(0, i + step))
    all[next]?.focus()
    all[next]?.scrollIntoView({ block: "nearest" })
  }
  const take = () => {
    const id = (document.activeElement as HTMLElement | null)?.dataset?.late
    if (!id) return
    bulk.mutate(
      { conversation_ids: [id], assignee_id: membership.member_id },
      { onSuccess: () => toast(t`Assigned to you`), onError: (e) => toast(errorText(e)) }
    )
  }
  useHotkeys({ [HOME_KEYS.next]: () => move(1), [HOME_KEYS.previous]: () => move(-1), [HOME_KEYS.take]: take })
}

function Overview({ h, inboxId }: { h: Home; inboxId: string }) {
  const { canManage } = useSession()
  const mine = useMine(inboxId) ?? []
  const late = useLate(inboxId)
  const waiting = useWaitingRows(h, inboxId, late)
  const later = useLaterRows(h, mine, inboxId)
  const products = useProductRows(h)
  const recent = useRecent(inboxId)
  return (
    <div className="grid items-start gap-4 lg:grid-cols-[minmax(0,1fr)_340px]">
      <div className="flex min-w-0 flex-col gap-4">
        {mine.length > 0 && <MyConversations mine={mine} />}
        {waiting.length > 0 && <WaitingForYou rows={waiting} />}
        <Today h={h} late={late?.length ?? 0} mine={mine} />
      </div>
      <div className="flex min-w-0 flex-col gap-4 empty:hidden">
        {later.length > 0 && <LaterToday rows={later} />}
        {canManage && products.length > 0 && <Products h={h} rows={products} />}
        {recent.length > 0 && <Recent items={recent} />}
      </div>
    </div>
  )
}

export function HomePage() {
  const { t } = useLingui()
  const { canManage, membership } = useSession()
  const [inboxId] = useInboxFilter()
  const home = useHome(true, { days: 7, inboxId })
  const h = home.data
  const [hidden, setHidden] = useSetupHidden()
  useRowKeys()
  const items = canManage && h?.setup ? cardItems(h, membership.workspace.focus) : []
  const open = items.some((i) => !i.done)
  const showCard = open && !hidden
  const fresh = canManage && !!h?.setup && h.setup.items.every((i) => !i.done)
  return (
    <PageLayout testId="home" label={t`Home`}>
      <div className="mx-auto flex w-full max-w-[1132px] flex-col gap-4 pt-6">
        <div className="flex flex-wrap items-center gap-x-4 gap-y-3">
          <Greeting />
          <div className="flex flex-wrap items-center gap-2">
            <Availability />
            {canManage && h && !showCard && (
              <Button
                variant="outline"
                className="h-9"
                render={<Link to="/setup" state={{ back: HOME_BACK } satisfies SetupState} />}
                data-testid="home-getting-started"
              >
                <Trans>Getting started</Trans>
              </Button>
            )}
          </div>
        </div>
        {home.error && (
          <Alert data-testid="home-error">
            <AlertDescription>
              <Trans>Couldn’t load Home. Your inbox still works.</Trans>
            </AlertDescription>
            <AlertAction>
              <Button size="sm" variant="outline" onClick={() => void home.refetch()}>
                <Trans>Try again</Trans>
              </Button>
            </AlertAction>
          </Alert>
        )}
        {!h ? (
          !home.error && <HomeSkeleton />
        ) : (
          <>
            {showCard && <SetupCard items={items} onHide={() => setHidden(true)} />}
            {fresh ? <WaitingForYou rows={[]} /> : <Overview h={h} inboxId={inboxId} />}
          </>
        )}
      </div>
    </PageLayout>
  )
}
