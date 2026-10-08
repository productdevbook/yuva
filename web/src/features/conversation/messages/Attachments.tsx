import { useLingui } from "@lingui/react/macro"
import { DownloadIcon, PaperclipIcon } from "lucide-react"

import { formatBytes } from "@/components/common/text"
import { attachmentUrl, type Attachment } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

const chip = "max-w-full rounded-xl border bg-card text-xs transition-colors hover:border-input"

function AttachmentChip({ a }: { a: Attachment }) {
  const { t, i18n } = useLingui()
  const { workspaceId } = useSession()
  const url = attachmentUrl(a, workspaceId)
  const size = formatBytes(a.size, i18n.locale)
  if (a.content_type.startsWith("image/") && a.content_type !== "image/svg+xml") {
    return (
      <a
        href={url}
        target="_blank"
        rel="noreferrer"
        className={cn(chip, "flex w-60 flex-col overflow-hidden")}
        title={t`Open ${a.filename}`}
        data-testid="attachment"
      >
        <img src={url} alt={a.filename} loading="lazy" className="max-h-44 w-full bg-surface object-contain" />
        <span className="flex items-center gap-2 px-2.5 py-1.5">
          <span className="min-w-0 truncate">{a.filename}</span>
          <span className="ms-auto shrink-0 text-faint">{size}</span>
        </span>
      </a>
    )
  }
  return (
    <a
      href={url}
      download={a.filename}
      className={cn(chip, "flex items-center gap-2 px-2.5 py-1.5")}
      title={t`Download ${a.filename}`}
      data-testid="attachment"
    >
      <PaperclipIcon className="size-3.5 shrink-0 text-faint" />
      <span className="min-w-0 truncate">{a.filename}</span>
      <span className="shrink-0 text-faint">{size}</span>
      <DownloadIcon className="size-3.5 shrink-0 text-faint" />
    </a>
  )
}

export function Attachments({ items, outgoing }: { items: Attachment[]; outgoing: boolean }) {
  if (items.length === 0) return null
  return (
    <div className={cn("flex max-w-full flex-col gap-1.5", outgoing && "items-end")}>
      {items.map((a) => (
        <AttachmentChip key={a.id} a={a} />
      ))}
    </div>
  )
}
