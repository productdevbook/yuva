import { Trans, useLingui } from "@lingui/react/macro"
import { ChevronRightIcon } from "lucide-react"
import { useEffect, useState } from "react"
import { Link } from "react-router"

import { ChannelIcon } from "@/components/common"
import { formatDate, useEnumText } from "@/components/common/text"
import { look } from "@/features/inbox/PageFrame"
import type { Channel } from "@/lib/api"
import { cn } from "@/lib/utils"

type Entry = { id: string; label: string }

export function useSectionEntries(root: HTMLElement | null, resetKey: string) {
  const [entries, setEntries] = useState<Entry[]>([])
  useEffect(() => {
    if (!root) return
    const read = () => {
      const next = Array.from(root.querySelectorAll<HTMLElement>("[data-testid^='section-']")).map((el) => ({
        id: el.id,
        label: el.querySelector("h2")?.textContent ?? el.id,
      }))
      setEntries((prev) =>
        prev.length === next.length && prev.every((p, i) => p.id === next[i]?.id && p.label === next[i]?.label)
          ? prev
          : next
      )
    }
    read()
    const watch = new MutationObserver(read)
    watch.observe(root, { childList: true, subtree: true, characterData: true })
    return () => watch.disconnect()
  }, [root, resetKey])
  return entries
}

function jump(id: string) {
  document.getElementById(id)?.scrollIntoView({ block: "start" })
}

export function ChannelTile({ kind }: { kind: Channel["kind"] }) {
  return (
    <span
      aria-hidden
      className="grid size-11 shrink-0 place-items-center rounded-xl bg-brand-wash text-brand shadow-[inset_0_0_0_1px_var(--border)]"
    >
      <ChannelIcon kind={kind} className="size-5" />
    </span>
  )
}

export function ChannelNav({ entries }: { entries: Entry[] }) {
  const { t } = useLingui()
  const [current, setCurrent] = useState("")
  if (entries.length === 0) return null
  const here = entries.some((e) => e.id === current) ? current : entries[0]!.id
  return (
    <>
      <nav className="sticky top-6 flex flex-col gap-px self-start @max-[759px]:hidden" aria-label={t`On this page`}>
        <h3 className={cn(look.over, "px-2.5 pb-1.5")}>
          <Trans>On this page</Trans>
        </h3>
        {entries.map((n) => (
          <a
            key={n.id}
            href={`#${n.id}`}
            aria-current={here === n.id ? "true" : undefined}
            onClick={(e) => {
              e.preventDefault()
              setCurrent(n.id)
              jump(n.id)
            }}
            className="flex h-[34px] items-center rounded-lg px-2.5 text-body text-muted-foreground outline-none hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring aria-[current=true]:bg-card aria-[current=true]:font-semibold aria-[current=true]:text-foreground aria-[current=true]:shadow-[0_0_0_1px_var(--border)]"
          >
            <span className="min-w-0 flex-1 truncate">{n.label}</span>
          </a>
        ))}
      </nav>
      <nav
        className="-mx-4 hidden gap-1.5 overflow-x-auto px-4 pb-1 [scrollbar-width:none] @max-[759px]:flex"
        aria-label={t`On this page`}
      >
        {entries.map((n) => (
          <a
            key={n.id}
            href={`#${n.id}`}
            onClick={(e) => {
              e.preventDefault()
              jump(n.id)
            }}
            className={look.listChip}
          >
            {n.label}
          </a>
        ))}
      </nav>
    </>
  )
}

type Fact = { key: string; label: React.ReactNode; value: React.ReactNode }

function useFacts(ch: Channel, productName: string | undefined): Fact[] {
  const { i18n } = useLingui()
  const text = useEnumText()
  const facts: Fact[] = [{ key: "type", label: <Trans>Type</Trans>, value: text.channel[ch.kind] }]
  if (productName) facts.push({ key: "product", label: <Trans>Product</Trans>, value: productName })
  facts.push({ key: "created", label: <Trans>Created</Trans>, value: formatDate(ch.created_at, i18n.locale) })
  return facts
}

export function FactsStrip({ ch, productName }: { ch: Channel; productName?: string }) {
  const facts = useFacts(ch, productName)
  return (
    <dl className="m-0 hidden flex-wrap gap-1.5 @max-[1239px]:flex" data-testid="channel-facts">
      {facts.map((f) => (
        <div key={f.key} className={cn(look.listChip, "h-auto min-h-7 max-w-full gap-1.5 py-0.5")}>
          <dt className="text-faint">{f.label}</dt>
          <dd className="m-0 min-w-0 truncate text-foreground">{f.value}</dd>
        </div>
      ))}
    </dl>
  )
}

export function ChannelAside({ ch, productName }: { ch: Channel; productName?: string }) {
  const facts = useFacts(ch, productName)
  const links = [
    { to: `/settings/inboxes/${ch.inbox_id}`, label: <Trans>Product settings</Trans> },
    { to: `/settings/inboxes/${ch.inbox_id}/webhooks`, label: <Trans>Webhooks</Trans> },
    { to: "/settings?view=channels", label: <Trans>Channels</Trans> },
  ]
  return (
    <aside
      className="sticky top-6 flex flex-col gap-4 self-start @max-[1239px]:hidden"
      aria-label={ch.name}
      data-testid="channel-aside"
    >
      <dl className="m-0 grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-2 px-2.5 text-[12.5px]">
        {facts.map((f) => (
          <div key={f.key} className="contents">
            <dt className="text-faint">{f.label}</dt>
            <dd className="m-0 text-end text-muted-foreground [overflow-wrap:anywhere]">{f.value}</dd>
          </div>
        ))}
      </dl>
      <div className="flex flex-col gap-1">
        <h3 className={cn(look.over, "px-2.5 pb-1")}>
          <Trans>Elsewhere in settings</Trans>
        </h3>
        {links.map((l) => (
          <Link
            key={l.to}
            to={l.to}
            className="flex h-[34px] items-center gap-2.5 rounded-lg px-2.5 text-body text-muted-foreground outline-none hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
          >
            <span className="min-w-0 flex-1 truncate">{l.label}</span>
            <ChevronRightIcon className="size-3.5 shrink-0 text-faint rtl:rotate-180" aria-hidden />
          </Link>
        ))}
      </div>
    </aside>
  )
}
