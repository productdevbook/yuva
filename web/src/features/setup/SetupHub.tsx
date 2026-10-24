import { plural } from "@lingui/core/macro"
import { Trans, useLingui } from "@lingui/react/macro"
import { BookOpenTextIcon, ChartLineIcon, CheckIcon, ChevronRightIcon, InboxIcon, MegaphoneIcon } from "lucide-react"
import { useState } from "react"
import { Link, Navigate, useLocation } from "react-router"

import { ChannelIcon, ErrorLine, StepHeader } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { PropertyCard } from "@/features/home/PropertyCard"
import { useHome } from "@/features/home/queries"
import { useKindText } from "@/features/setup/ChannelStep"
import { DEFAULT_COLOR, KindPreview, MailMock, PreviewCaption, SubscribeMock } from "@/features/setup/previews"
import { orderByFocus, startPath, type HubId, type SetupState } from "@/features/setup/setup"
import { SetupLayout } from "@/features/setup/SetupLayout"
import type { HomeSetupItem } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useChannelMap, useInboxes } from "@/lib/workspace"

export function useHubText() {
  const { t } = useLingui()
  const kinds = useKindText()
  return {
    title: {
      chat: kinds.chat.title,
      email: kinds.email.title,
      mail: t`Team mail`,
      app: kinds.app.title,
      docs: kinds.docs.title,
      analytics: t`Analytics`,
      newsletter: t`Newsletter`,
    } satisfies Record<HubId, string>,
    hint: {
      chat: kinds.chat.hint,
      email: kinds.email.hint,
      mail: t`A shared mailbox like hello@ or billing@ for your team`,
      app: kinds.app.hint,
      docs: kinds.docs.hint,
      analytics: t`Visitors of your website or app, without cookies`,
      newsletter: t`A mailing list with subscribe forms, for your broadcasts`,
    } satisfies Record<HubId, string>,
    count: (id: HubId, n: number) =>
      ({
        chat: plural(n, { one: "# website chat", other: "# website chats" }),
        email: plural(n, { one: "# e-mail address", other: "# e-mail addresses" }),
        mail: plural(n, { one: "# team mailbox", other: "# team mailboxes" }),
        app: plural(n, { one: "# app", other: "# apps" }),
        docs: plural(n, { one: "# documentation site", other: "# documentation sites" }),
        analytics: plural(n, { one: "# website or app", other: "# websites and apps" }),
        newsletter: plural(n, { one: "# mailing list", other: "# mailing lists" }),
      })[id],
  }
}

export function useHubPlaces(): Record<HubId, string> {
  const channels = [...useChannelMap().values()]
  const settingsOf = (kind: "chat" | "email" | "app") => {
    const ch = channels.find((c) => c.kind === kind)
    return ch ? `/settings/inboxes/${ch.inbox_id}` : "/settings"
  }
  return {
    chat: settingsOf("chat"),
    email: settingsOf("email"),
    mail: "/mail",
    app: settingsOf("app"),
    docs: "/docs",
    analytics: "/analytics",
    newsletter: "/broadcasts/lists",
  }
}

export function hubTarget(item: HomeSetupItem, places: Record<HubId, string>) {
  return item.done ? places[item.id] : startPath(item.id)
}

function HubIcon({ id }: { id: HubId }) {
  if (id === "docs") return <BookOpenTextIcon />
  if (id === "analytics") return <ChartLineIcon />
  if (id === "newsletter") return <MegaphoneIcon />
  if (id === "mail") return <InboxIcon />
  return <ChannelIcon kind={id} />
}

function HubPreview({ id }: { id: HubId }) {
  const { t } = useLingui()
  const { membership } = useSession()
  const inbox = useInboxes().data?.[0]
  const name = inbox?.name ?? membership.workspace.name
  const color = inbox?.branding.color ?? DEFAULT_COLOR
  const caption = {
    chat: <Trans>Visitors write from any page of your site; you answer here, live.</Trans>,
    email: <Trans>Customers keep writing to the address they know.</Trans>,
    mail: <Trans>Your team shares mailboxes like hello@ or billing@, kept out of support views and stats.</Trans>,
    app: <Trans>People write from inside your app, signed in as themselves.</Trans>,
    docs: <Trans>Readers rate each page, send feedback and discuss it in public; you answer and moderate here.</Trans>,
    analytics: <Trans>Every website and app shows its visitors on Home and in Analytics.</Trans>,
    newsletter: <Trans>People subscribe on your site; broadcasts go to the list.</Trans>,
  }[id]
  return (
    <>
      {id === "analytics" ? (
        <PropertyCard example name={t`${name} website`} host="www.example.com" />
      ) : id === "newsletter" ? (
        <SubscribeMock name={t`${name} news`} color={color} />
      ) : id === "mail" ? (
        <MailMock name={name} to="hello@example.com" />
      ) : (
        <KindPreview kind={id} name={name} color={color} host="www.example.com" />
      )}
      <PreviewCaption>{caption}</PreviewCaption>
    </>
  )
}

export function SetupHub() {
  const { canManage, membership } = useSession()
  const state = (useLocation().state ?? {}) as SetupState
  const home = useHome(canManage)
  const text = useHubText()
  const places = useHubPlaces()
  const [focus, setFocus] = useState<HubId>("chat")
  if (!canManage) return <Navigate to="/" replace />
  const items = home.data?.setup && orderByFocus(home.data.setup.items, membership.workspace.focus)
  const left = items?.filter((i) => !i.done).length ?? 0
  return (
    <SetupLayout
      preview={<HubPreview id={focus} />}
      leave={
        <Button variant="ghost" size="sm" render={<Link to={state.back ?? "/"} replace />} data-testid="setup-later">
          {left > 0 ? <Trans>Do this later</Trans> : <Trans>Close</Trans>}
        </Button>
      }
    >
      <StepHeader title={<Trans>Set up Yuva</Trans>}>
        {items && left === 0 ? (
          <Trans>Everything is set up. Open any part to change it, or add another from its settings.</Trans>
        ) : (
          <Trans>
            Pick what you want to use. Each part takes a few minutes, and you can come back here at any time.
          </Trans>
        )}
      </StepHeader>
      <ErrorLine error={home.error} />
      {!items ? (
        !home.error && (
          <div className="flex flex-col gap-2.5">
            {Array.from({ length: 6 }, (_, i) => (
              <Skeleton key={i} className="h-[68px] rounded-2xl" />
            ))}
          </div>
        )
      ) : (
        <ul className="flex flex-col gap-2.5" data-testid="setup-hub">
          {items.map((item) => (
            <li key={item.id}>
              <Link
                to={hubTarget(item, places)}
                state={item.done ? undefined : ({ back: "/setup" } satisfies SetupState)}
                className={cn(
                  "group flex w-full items-center gap-3.5 rounded-2xl border bg-card px-4 py-3.5 text-start transition-colors outline-none hover:border-faint focus-visible:ring-3 focus-visible:ring-ring/30",
                  focus === item.id && "border-faint"
                )}
                onMouseEnter={() => setFocus(item.id)}
                onFocus={() => setFocus(item.id)}
                data-testid={`setup-card-${item.id}`}
                data-done={item.done || undefined}
              >
                <span
                  className={cn(
                    "grid size-9 shrink-0 place-items-center rounded-[10px] border bg-surface text-muted-foreground [&_svg]:size-4",
                    item.done && "border-success/20 bg-success/10 text-success"
                  )}
                >
                  {item.done ? <CheckIcon /> : <HubIcon id={item.id} />}
                </span>
                <span className="min-w-0 flex-1">
                  <span className="block font-medium">{text.title[item.id]}</span>
                  <span className="mt-0.5 block text-small text-faint">
                    {item.done ? text.count(item.id, item.count) : text.hint[item.id]}
                  </span>
                </span>
                {item.done && (
                  <span className="shrink-0 text-small font-medium text-success">
                    <Trans>Done</Trans>
                  </span>
                )}
                <ChevronRightIcon className="size-4 shrink-0 text-faint transition-transform group-hover:translate-x-0.5 rtl:rotate-180" />
              </Link>
            </li>
          ))}
        </ul>
      )}
    </SetupLayout>
  )
}
