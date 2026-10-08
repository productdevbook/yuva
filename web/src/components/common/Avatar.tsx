import { useState } from "react"

import { initials } from "@/components/common/text"
import { cn } from "@/lib/utils"

const avatarClass = "inline-flex size-7 shrink-0 items-center justify-center rounded-full bg-muted text-[11px] font-medium text-muted-foreground"

export function PersonAvatar({ name, className }: { name: string; className?: string }) {
  return (
    <span aria-hidden className={cn(avatarClass, className)}>
      {initials(name)}
    </span>
  )
}

export function BotAvatar({ name, url, className }: { name: string; url?: string; className?: string }) {
  const [failed, setFailed] = useState<string | null>(null)
  if (!url || failed === url) return <PersonAvatar name={name} className={className} />
  return (
    <img
      src={url}
      alt=""
      aria-hidden
      referrerPolicy="no-referrer"
      loading="lazy"
      onError={() => setFailed(url)}
      className={cn(avatarClass, "object-cover", className)}
    />
  )
}
