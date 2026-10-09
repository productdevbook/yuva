import { initials } from "@/components/common/text"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { cn } from "@/lib/utils"

const rootClass = "@container size-7 items-center justify-center bg-muted font-medium text-muted-foreground after:hidden"
const fallbackClass = "bg-transparent text-[length:38cqw] font-[inherit] text-inherit"

export function PersonAvatar({ name, className }: { name: string; className?: string }) {
  return (
    <Avatar aria-hidden className={cn(rootClass, className)}>
      <AvatarFallback className={fallbackClass}>{initials(name)}</AvatarFallback>
    </Avatar>
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
    <Avatar
      aria-hidden
      className={cn(rootClass, "text-white", className)}
      style={{ backgroundColor: colorFor(id) }}
    >
      <AvatarFallback className={fallbackClass}>{initials(name)}</AvatarFallback>
    </Avatar>
  )
}

export function BotAvatar({ name, url, className }: { name: string; url?: string; className?: string }) {
  return (
    <Avatar aria-hidden className={cn(rootClass, className)}>
      {url && <AvatarImage src={url} alt="" referrerPolicy="no-referrer" loading="lazy" />}
      <AvatarFallback className={fallbackClass}>{initials(name)}</AvatarFallback>
    </Avatar>
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
      <Avatar aria-hidden className={cn(rootClass, "size-[26px] bg-mate text-white", !here && online !== undefined && "opacity-55", className)}>
        <AvatarFallback className={fallbackClass}>{initials(name)}</AvatarFallback>
      </Avatar>
      {online !== undefined && (
        <span className={cn("absolute -end-px -bottom-px size-2 rounded-full ring-2", ring, here ? "bg-success" : "bg-faint")} />
      )}
    </span>
  )
}
