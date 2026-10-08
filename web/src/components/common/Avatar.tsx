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

const palette = ["#d4431c", "#2563eb", "#059669", "#7c3aed", "#db2777", "#0891b2"]

export function colorFor(id: string) {
  let h = 2166136261
  for (let i = 0; i < id.length; i++) h = Math.imul(h ^ id.charCodeAt(i), 16777619)
  return palette[(h >>> 0) % palette.length]
}

export function ContactAvatar({ id, name, className }: { id: string; name: string; className?: string }) {
  return (
    <span
      aria-hidden
      className={cn("inline-flex size-7 shrink-0 items-center justify-center rounded-full text-[11px] font-semibold tracking-tight text-white", className)}
      style={{ backgroundColor: colorFor(id) }}
    >
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

export function MemberAvatar({
  name,
  online,
  away,
  className,
  ring = "ring-background",
}: {
  name: string
  online?: boolean
  away?: boolean
  className?: string
  ring?: string
}) {
  const here = online && !away
  return (
    <span className="relative inline-flex shrink-0">
      <span
        aria-hidden
        className={cn(
          "inline-flex size-[26px] items-center justify-center rounded-full bg-mate text-[10px] font-semibold text-white",
          !here && "opacity-55",
          className,
        )}
      >
        {initials(name)}
      </span>
      {online !== undefined && (
        <span className={cn("absolute -end-px -bottom-px size-2 rounded-full ring-2", ring, here ? "bg-green-600" : "bg-zinc-400")} />
      )}
    </span>
  )
}
