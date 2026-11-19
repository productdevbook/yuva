import { Trans, useLingui } from "@lingui/react/macro"
import { BellIcon, ChevronsUpDownIcon, SearchIcon } from "lucide-react"
import { Link, useNavigate } from "react-router"

import { usePage, usePageGroups } from "@/app/nav"
import { useShell } from "@/app/shell"
import { UserMenu } from "@/app/UserMenu"
import { YuvaMark } from "@/components/common"
import { BackArrow, usePhoneBack } from "@/components/common/PageLayout"
import { keyLabel, mod, SHORTCUTS } from "@/components/common/ShortcutSheet"
import { WorkspaceSelector } from "@/components/common/WorkspaceSelector"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Kbd, KbdGroup } from "@/components/ui/kbd"
import { messageLink, useAuthorLabel } from "@/features/inbox/SidePanels"
import { useMarkAllMentionsSeen, useMarkMentionSeen, useMentions } from "@/features/inbox/queries"
import { setRailView } from "@/features/inbox/railView"
import { useIsMobile } from "@/hooks/use-media-query"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

const MENTIONS_SHOWN = 6

function PagesMenu() {
  const { t } = useLingui()
  const groups = usePageGroups()
  const page = usePage()
  const current = groups.flatMap((g) => g.entries).find((e) => e.key === page)
  const name = current?.label ?? t`Inbox`
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        className="flex h-7 max-w-60 min-w-0 shrink items-center gap-1.5 rounded-md px-1.5 text-small font-semibold text-foreground outline-none transition-colors hover:bg-raise focus-visible:ring-2 focus-visible:ring-ring data-popup-open:bg-raise max-sm:max-w-44"
        aria-label={t`Page: ${name}`}
        data-testid="pages-menu"
      >
        <span className="min-w-0 truncate" data-testid="page-name">
          {name}
        </span>
        <ChevronsUpDownIcon aria-hidden className="size-3.5 shrink-0 text-muted-foreground" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="max-h-[calc(100svh-4rem)] w-72" data-testid="pages-menu-list">
        {groups.map((g, i) => (
          <DropdownMenuGroup key={g.key}>
            {i > 0 && <DropdownMenuSeparator />}
            <DropdownMenuLabel>{g.title}</DropdownMenuLabel>
            {g.entries.map((e) => (
              <DropdownMenuItem
                key={e.key}
                onClick={e.open}
                aria-current={e.key === page ? "page" : undefined}
                aria-label={e.count ? e.countLabel : undefined}
                className="aria-[current=page]:bg-brand-wash aria-[current=page]:font-semibold"
                data-testid={`pages-menu-${e.key}`}
              >
                <span className="min-w-0 flex-1 truncate">{e.label}</span>
                {!!e.count && (
                  <span className="text-caption font-medium text-brand tabular-nums">
                    {e.count > 99 ? "99+" : e.count}
                  </span>
                )}
                {e.keys.length > 0 && <DropdownMenuShortcut>{e.keys.join(" ")}</DropdownMenuShortcut>}
              </DropdownMenuItem>
            ))}
          </DropdownMenuGroup>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function SearchButton() {
  const { t } = useLingui()
  const { openPalette } = useShell()
  const label = t`Search or jump to…`
  return (
    <Button
      variant="outline"
      onClick={openPalette}
      aria-label={`${label} ${mod} K`}
      title={label}
      className="me-0.5 h-7 w-[230px] justify-start gap-1.5 rounded-md bg-card ps-2 pe-1.5 text-small font-normal text-muted-foreground max-[899px]:w-7 max-[899px]:justify-center max-[899px]:border-transparent max-[899px]:bg-transparent max-[899px]:p-0"
      data-testid="header-search"
    >
      <SearchIcon aria-hidden className="size-[15px]" />
      <span className="min-w-0 flex-1 truncate text-start max-[899px]:hidden">{label}</span>
      <KbdGroup aria-hidden className="max-[899px]:hidden">
        <Kbd className="h-[18px] border bg-background px-1 font-mono text-[10.5px]">{mod}</Kbd>
        <Kbd className="h-[18px] border bg-background px-1 font-mono text-[10.5px]">K</Kbd>
      </KbdGroup>
    </Button>
  )
}

function Bell() {
  const { t } = useLingui()
  const navigate = useNavigate()
  const mentions = useMentions()
  const seen = useMarkMentionSeen()
  const markAll = useMarkAllMentionsSeen()
  const author = useAuthorLabel()
  const unseen = Number(mentions.data?.pages[0]?.unseen ?? 0)
  const items = (mentions.data?.pages.flatMap((p) => p.items) ?? []).slice(0, MENTIONS_SHOWN)
  const label = unseen ? t`Mentions, ${unseen} unseen` : t`Mentions`
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={<Button variant="ghost" size="icon-sm" className="relative size-7 text-fg2" />}
        aria-label={label}
        title={label}
        data-testid="header-bell"
      >
        <BellIcon className="size-[17px]" />
        {unseen > 0 && (
          <span
            aria-hidden
            className="absolute end-[5px] top-[5px] size-[7px] rounded-full bg-brand ring-2 ring-background"
            data-testid="header-bell-dot"
          />
        )}
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-[340px] max-sm:w-[calc(100vw-1rem)]" data-testid="header-bell-menu">
        <DropdownMenuGroup>
          <DropdownMenuLabel>
            <Trans>Mentions</Trans>
          </DropdownMenuLabel>
          {mentions.data && items.length === 0 && (
            <p className="m-0 px-2 pb-2 text-small text-muted-foreground">
              <Trans>Nobody has mentioned you yet</Trans>
            </p>
          )}
          {items.map((m) => {
            const who = author(m.note.author)
            const where = m.conversation.subject || m.conversation.contact.name || m.conversation.contact.email || ""
            return (
              <DropdownMenuItem
                key={m.note.id}
                onClick={() => {
                  if (!m.seen) seen.mutate(m.note.id)
                  navigate(messageLink(m.conversation.id, m.note.id))
                }}
                className="items-start gap-2.5 py-2"
                data-testid="header-mention"
                data-seen={m.seen}
              >
                <span
                  aria-hidden
                  className={cn("mt-1.5 size-2 shrink-0 rounded-full", m.seen ? "bg-transparent" : "bg-brand")}
                />
                <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                  <span className={cn("truncate text-small", !m.seen && "font-semibold")}>
                    <Trans>
                      {who} in {where}
                    </Trans>
                  </span>
                  <span className="line-clamp-2 text-caption text-muted-foreground">{m.note.text}</span>
                </span>
              </DropdownMenuItem>
            )
          })}
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          onClick={() => {
            setRailView("mentions")
            navigate("/mentions")
          }}
          data-testid="header-mentions-all"
        >
          <Trans>All mentions</Trans>
          <DropdownMenuShortcut>{keyLabel(SHORTCUTS.mentions).join(" ")}</DropdownMenuShortcut>
        </DropdownMenuItem>
        {unseen > 0 && (
          <DropdownMenuItem onClick={() => markAll.mutate()} data-testid="header-mentions-read">
            <Trans>Mark all as read</Trans>
          </DropdownMenuItem>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function PhoneBack() {
  const back = usePhoneBack()
  const mobile = useIsMobile()
  if (!back || !mobile) return null
  const className = "size-7 text-foreground"
  if (back.to === undefined)
    return (
      <Button
        variant="ghost"
        size="icon-sm"
        onClick={back.run}
        aria-label={back.label}
        className={className}
        data-testid="top-back"
      >
        <BackArrow className="size-[18px]" />
      </Button>
    )
  return (
    <Button
      variant="ghost"
      size="icon-sm"
      render={<Link to={back.to} onClick={back.run} />}
      aria-label={back.label}
      className={className}
      data-testid="top-back"
    >
      <BackArrow className="size-[18px]" />
    </Button>
  )
}

export function Header() {
  const { t } = useLingui()
  const { homePath } = useSession()
  return (
    <header
      className="relative z-20 flex h-10 shrink-0 items-center gap-0.5 border-b border-line-soft bg-background px-2.5 max-sm:px-1.5"
      data-testid="header"
    >
      <PhoneBack />
      <Link
        to={homePath}
        aria-label={t`Yuva, go home`}
        className="grid size-7 shrink-0 place-items-center rounded-[7px] outline-none hover:bg-raise focus-visible:ring-2 focus-visible:ring-ring"
        data-testid="header-home"
      >
        <YuvaMark className="size-[22px]" />
      </Link>
      <WorkspaceSelector />
      <span aria-hidden className="shrink-0 px-px text-[15px] font-light text-input select-none">
        /
      </span>
      <PagesMenu />
      <span className="min-w-1 flex-1" />
      <SearchButton />
      <Bell />
      <UserMenu />
    </header>
  )
}
