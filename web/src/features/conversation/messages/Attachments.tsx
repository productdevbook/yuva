import { useLingui } from "@lingui/react/macro"
import { DownloadIcon, PaperclipIcon } from "lucide-react"

import { formatBytes } from "@/components/common/text"
import {
  Attachment,
  AttachmentActions,
  AttachmentContent,
  AttachmentDescription,
  AttachmentGroup,
  AttachmentMedia,
  AttachmentTitle,
  AttachmentTrigger,
} from "@/components/ui/attachment"
import { attachmentUrl, type Attachment as AttachmentItem } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

export function isPreviewable(a: AttachmentItem) {
  return a.content_type.startsWith("image/") && a.content_type !== "image/svg+xml"
}

function AttachmentChip({ a }: { a: AttachmentItem }) {
  const { t, i18n } = useLingui()
  const { workspaceId } = useSession()
  const url = attachmentUrl(a, workspaceId)
  const size = formatBytes(a.size, i18n.locale)
  if (isPreviewable(a)) {
    return (
      <Attachment orientation="vertical" className="w-44 bg-card">
        <AttachmentMedia variant="image" className="bg-surface">
          <img src={url} alt={a.filename} loading="lazy" />
        </AttachmentMedia>
        <AttachmentContent>
          <AttachmentTitle className="text-caption">{a.filename}</AttachmentTitle>
          <AttachmentDescription className="text-faint">{size}</AttachmentDescription>
        </AttachmentContent>
        <AttachmentTrigger
          render={<a href={url} target="_blank" rel="noreferrer" />}
          aria-label={t`Open ${a.filename}`}
          title={t`Open ${a.filename}`}
          data-testid="attachment"
        />
      </Attachment>
    )
  }
  return (
    <Attachment size="sm" className="max-w-72 bg-card">
      <AttachmentMedia>
        <PaperclipIcon className="text-faint" />
      </AttachmentMedia>
      <AttachmentContent>
        <AttachmentTitle>{a.filename}</AttachmentTitle>
        <AttachmentDescription className="text-faint">{size}</AttachmentDescription>
      </AttachmentContent>
      <AttachmentActions>
        <DownloadIcon className="size-3.5 text-faint" />
      </AttachmentActions>
      <AttachmentTrigger
        render={<a href={url} download={a.filename} />}
        aria-label={t`Download ${a.filename}`}
        title={t`Download ${a.filename}`}
        data-testid="attachment"
      />
    </Attachment>
  )
}

export function Attachments({ items, outgoing, className }: { items: AttachmentItem[]; outgoing?: boolean; className?: string }) {
  if (items.length === 0) return null
  return (
    <AttachmentGroup className={cn("flex-wrap overflow-visible", outgoing && "justify-end", className)} data-testid="attachments">
      {items.map((a) => (
        <AttachmentChip key={a.id} a={a} />
      ))}
    </AttachmentGroup>
  )
}
