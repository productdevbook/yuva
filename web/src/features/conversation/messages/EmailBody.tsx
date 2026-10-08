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

export function EmailBody({ m, outgoing, expandQuoted }: { m: Message; outgoing: boolean; expandQuoted: boolean }) {
  const { t } = useLingui()
  const { workspaceId } = useSession()
  const [quotedOwn, setQuotedOwn] = useState<boolean | null>(null)
  const [images, setImages] = useState(false)
  useEffect(() => setQuotedOwn(null), [expandQuoted])
  const quotedShown = !!m.email?.quoted && (quotedOwn ?? expandQuoted)
  const detail = useMessageEmail(m.id, quotedShown)
  const full = quotedShown ? detail.data : undefined
  const html = full ? full.full_html : m.html
  const text = full ? full.full_text : m.body
  const remote = !!html && !!(full ? full.has_remote_images : m.email?.has_remote_images)
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
      {html ? (
        <div className="w-full overflow-hidden rounded-2xl border bg-white">
          <EmailHtml html={html} images={images} inline={inline} />
        </div>
      ) : (
        text && <Bubble tone={outgoing ? "out" : "in"}>{text}</Bubble>
      )}
      {quotedShown && detail.isPending && <Skeleton className="h-10 w-full" />}
      <ErrorLine error={quotedShown && detail.error} className="text-xs" />
      {(m.email?.quoted || (remote && !images)) && (
        <div className={cn("flex flex-wrap gap-1", outgoing && "justify-end")}>
          {m.email?.quoted && (
            <Button
              variant="ghost"
              size="xs"
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
              size="xs"
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
