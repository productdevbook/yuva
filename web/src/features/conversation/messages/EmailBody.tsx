import { Trans, useLingui } from "@lingui/react/macro"
import { useEffect, useMemo, useRef, useState } from "react"

import { ErrorLine } from "@/components/common"
import { EmailHtml, knownHeight, placedContentIds, useDark } from "@/components/common/EmailHtml"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { Attachments } from "@/features/conversation/messages/Attachments"
import { Bubble } from "@/features/conversation/messages/Bubble"
import { useMessageEmail } from "@/features/conversation/queries"
import { useNear } from "@/hooks/use-near"
import { attachmentUrl, type Message } from "@/lib/api"
import { useEmailView } from "@/lib/emailView"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

const ORIGINAL_HEIGHT = 240

function isInlineImage(a: Message["attachments"][number]) {
  return !!a.content_id && a.content_type.startsWith("image/") && a.content_type !== "image/svg+xml"
}

export function EmailBody({ m, outgoing, expandQuoted, plain }: { m: Message; outgoing: boolean; expandQuoted: boolean; plain?: boolean }) {
  const { t } = useLingui()
  const { workspaceId } = useSession()
  const [quotedOwn, setQuotedOwn] = useState<boolean | null>(null)
  const [images, setImages] = useState(false)
  const [originalOwn, setOriginalOwn] = useState<boolean | null>(null)
  const [asSent, setAsSent] = useState(false)
  const dark = useDark()
  const preferred = useEmailView()
  const slot = useRef<HTMLDivElement>(null)
  useEffect(() => setQuotedOwn(null), [expandQuoted])
  const quotedWanted = !!m.email?.quoted && (quotedOwn ?? expandQuoted)
  const wantOriginal = !!m.html && !!m.email?.raw && (originalOwn ?? preferred === "original")
  const near = useNear(slot, wantOriginal && originalOwn === null)
  const detail = useMessageEmail(m.id, quotedWanted || (wantOriginal && (near || originalOwn !== null)))
  const waiting = wantOriginal && !detail.data && !detail.error
  const originalHtml = wantOriginal ? detail.data?.original_html : undefined
  const canOriginal = !!m.html && !!m.email?.raw && !(detail.data && !detail.data.original_html)
  const quotedShown = quotedWanted && !originalHtml
  const heightKey = `${m.id}:original`
  const full = quotedShown || originalHtml ? detail.data : undefined
  const html = originalHtml ?? (full ? full.full_html : m.html)
  const text = full ? full.full_text : m.body
  const remote = !!html && !!(full ? full.has_remote_images : m.email?.has_remote_images)
  const pending = quotedShown && !waiting && detail.isPending
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
      {waiting ? (
        <div ref={slot} className="w-full" data-testid="original-loading">
          <Skeleton className="w-full rounded-xl" style={{ height: knownHeight(heightKey) ?? ORIGINAL_HEIGHT }} />
        </div>
      ) : originalHtml ? (
        <div className={cn("w-full overflow-hidden rounded-xl border", dark && !asSent ? "bg-card" : "bg-white")}>
          <EmailHtml
            html={originalHtml}
            images={images}
            inline={inline}
            original
            asSent={asSent}
            heightKey={heightKey}
            initialHeight={ORIGINAL_HEIGHT}
          />
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
      <ErrorLine error={(quotedShown || wantOriginal) && detail.error} className="text-caption" />
      {!waiting && (m.email?.quoted || canOriginal || (remote && !images)) && (
        <div className={cn("flex flex-wrap gap-1", plain ? "-ms-2 mt-1" : outgoing && "justify-end")}>
          {canOriginal && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setOriginalOwn(!originalHtml)
                if (!originalHtml && detail.error) void detail.refetch()
              }}
              aria-pressed={!!originalHtml}
              data-testid="original-toggle"
            >
              {originalHtml ? <Trans>Show reading view</Trans> : <Trans>Show original</Trans>}
            </Button>
          )}
          {originalHtml && dark && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setAsSent(!asSent)}
              aria-pressed={asSent}
              title={asSent ? t`Darken the colours to match the theme` : t`Show the colours the sender chose, on white`}
              data-testid="as-sent-toggle"
            >
              {asSent ? <Trans>Darken</Trans> : <Trans>Show as sent</Trans>}
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
