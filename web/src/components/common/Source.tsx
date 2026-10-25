import { useLingui } from "@lingui/react/macro"
import { CodeXmlIcon, MailIcon, MessageCircleIcon, MessageSquareMoreIcon, SmartphoneIcon } from "lucide-react"

import type { Channel, ChannelKind, Conversation } from "@/lib/api"
import { cn } from "@/lib/utils"

export type Source = ChannelKind | "feedback"

export const SOURCE_FILTERS: ChannelKind[] = ["chat", "email", "app"]

const icons: Record<Source, React.ComponentType<{ className?: string }>> = {
  chat: MessageCircleIcon,
  email: MailIcon,
  app: SmartphoneIcon,
  api: CodeXmlIcon,
  feedback: MessageSquareMoreIcon,
}

export function useSourceText(): Record<Source, string> {
  const { t } = useLingui()
  return {
    chat: t`Live chat`,
    email: t`E-mail`,
    app: t`App`,
    api: t`API`,
    feedback: t`Feedback`,
  }
}

export function sourceOf(c: Conversation, channel?: Channel): Source | undefined {
  if (c.kind === "feedback") return c.kind
  return c.channel_kind ?? channel?.kind
}

export function pageOf(c: Pick<Conversation, "page" | "feedback">) {
  return c.page?.page_url ?? c.feedback?.page_url
}

export function originOf(url: string) {
  try {
    return new URL(url).origin
  } catch {
    return ""
  }
}

export function hostOf(url: string) {
  try {
    return new URL(url).host
  } catch {
    return url
  }
}

export function hostPath(url: string) {
  try {
    const u = new URL(url)
    return u.host + (u.pathname === "/" ? "" : u.pathname)
  } catch {
    return url
  }
}

export function placeOf(c: Conversation, channel?: Channel) {
  const page = pageOf(c)
  if (page) return hostPath(page)
  return channel?.kind === "chat" || channel?.kind === "app" ? channel.name : undefined
}

export function isVisitor(name?: string, email?: string) {
  return !name?.trim() && !email
}

export function useContactName() {
  const { t } = useLingui()
  return (c: Pick<Conversation, "page" | "feedback">, channel: Channel | undefined, name?: string, email?: string) => {
    if (!isVisitor(name, email)) return name?.trim() || email || ""
    const page = pageOf(c)
    const where = page ? hostOf(page) : channel?.kind === "chat" ? channel.name : undefined
    return where ? t`Visitor · ${where}` : t`Visitor`
  }
}

export function SourceIcon({ source, className }: { source?: Source; className?: string }) {
  const Icon = icons[source ?? "chat"]
  return <Icon className={className} />
}

export function SourceBadge({ source, className }: { source: Source; className?: string }) {
  const text = useSourceText()
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center gap-1 rounded-full bg-muted px-1.5 py-px text-caption font-medium text-muted-foreground",
        className
      )}
      data-testid="source-badge"
      data-source={source}
    >
      <SourceIcon source={source} className="size-3 shrink-0" />
      {text[source]}
    </span>
  )
}
