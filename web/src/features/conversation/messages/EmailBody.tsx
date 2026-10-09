import { Trans, useLingui } from "@lingui/react/macro"
import { useEffect, useMemo, useState } from "react"

import { ErrorLine } from "@/components/common"
import { EmailHtml, placedContentIds } from "@/components/common/EmailHtml"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { Attachments } from "@/features/conversation/messages/Attachments"
import { Bubble } from "@/features/conversation/messages/Bubble"
import { useMessageEmail } from "@/features/conversation/queries"
import { attachmentUrl, type Message } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

function isInlineImage(a: Message["attachments"][number]) {
  return !!a.content_id && a.content_type.startsWith("image/") && a.content_type !== "image/svg+xml"
}

export function EmailBody({ m, outgoing, expandQuoted, plain }: { m: Message; outgoing: boolean; expandQuoted: boolean; plain?: boolean }) {
  const { t } = useLingui()
  const { workspaceId } = useSession()
  const [quotedOwn, setQuotedOwn] = useState<boolean | null>(null)
  const [images, setImages] = useState(false)
  const [originalOwn, setOriginalOwn] = useState(false)
  useEffect(() => setQuotedOwn(null), [expandQuoted])
  const quotedWanted = !!m.email?.quoted && (quotedOwn ?? expandQuoted)
  const detail = useMessageEmail(m.id, quotedWanted || originalOwn)
  const originalHtml = originalOwn ? detail.data?.original_html : undefined
  const canOriginal = !!m.html && !!m.email?.raw && !(detail.data && !detail.data.original_html)
  const quotedShown = quotedWanted && !originalHtml
  const full = quotedShown || originalHtml ? detail.data : undefined
  const html = originalHtml ?? (full ? full.full_html : m.html)
  const text = full ? full.full_text : m.body
  const remote = !!html && !!(full ? full.has_remote_images : m.email?.has_remote_images)
  const pending = (quotedShown || (originalOwn && canOriginal)) && detail.isPending
  const inline = useMemo(
    () => new Map(m.attachments.filter(isInlineImage).map((a) => [a.content_id!, attachmentUrl(a, workspaceId)])),
    [m.attachments, workspaceId],
  )
  const listed = useMemo(() => {
    const placed = html && inline.size > 0 ? placedContentIds(html) : new Set<string>()
    return m.attachments.filter((a) => !(a.inline && a.content_id && inline.has(a.content_id) && placed.has(a.content_id)))
  }, [html, inline, m.attachments])
  return (
    <>
      {originalHtml ? (
        <div className="w-full overflow-hidden rounded-xl border bg-white">
          <EmailHtml html={originalHtml} images={images} inline={inline} original />
        </div>
      ) : html ? (
        plain ? (
          <EmailHtml html={html} images={images} inline={inline} />
        ) : (
          <div className="w-full rounded-[18px] border bg-card px-[15px] py-[11px]">
            <EmailHtml html={html} images={images} inline={inline} />
          </div>
        )
      ) : plain ? (
        text && <div className="text-reading break-words whitespace-pre-wrap">{text}</div>
      ) : (
        text && <Bubble tone={outgoing ? "out" : "in"}>{text}</Bubble>
      )}
      {pending && <Skeleton className="h-10 w-full" />}
      <ErrorLine error={(quotedShown || originalOwn) && detail.error} className="text-caption" />
      {(m.email?.quoted || canOriginal || (remote && !images)) && (
        <div className={cn("flex flex-wrap gap-1", plain ? "-ms-2 mt-1" : outgoing && "justify-end")}>
          {canOriginal && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setOriginalOwn(!originalOwn)}
              aria-pressed={originalOwn}
              data-testid="original-toggle"
            >
              {originalOwn ? <Trans>Show reading view</Trans> : <Trans>Show original</Trans>}
            </Button>
          )}
          {m.email?.quoted && !originalHtml && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setQuotedOwn(!quotedShown)}
              aria-expanded={quotedShown}
              data-testid="quoted-toggle"
            >
              {quotedShown ? <Trans>Hide quoted text</Trans> : <Trans>Show quoted text</Trans>}
            </Button>
          )}
          {remote && !images && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setImages(true)}
              title={t`Remote images can tell the sender that you opened the mail.`}
              data-testid="load-images"
            >
              <Trans>Load images</Trans>
            </Button>
          )}
        </div>
      )}
      <Attachments items={listed} outgoing={outgoing} />
    </>
  )
}
