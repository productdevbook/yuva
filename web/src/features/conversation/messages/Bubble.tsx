import { cn } from "@/lib/utils"

const tones = {
  in: "rounded-ss-md bg-muted",
  out: "rounded-se-md bg-brand-wash",
  note: "border border-dashed border-input",
  draft: "rounded-se-md border border-dashed border-warning/50 bg-warning/5",
}

export function Bubble({ tone, children }: { tone: keyof typeof tones; children: React.ReactNode }) {
  return (
    <div className={cn("max-w-full rounded-2xl px-3.5 py-2.5 text-sm leading-relaxed break-words whitespace-pre-wrap", tones[tone])}>
      {children}
    </div>
  )
}
