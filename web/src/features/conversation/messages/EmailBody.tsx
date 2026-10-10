import { Trans, useLingui } from "@lingui/react/macro"
import { useEffect, useMemo, useRef, useState } from "react"

import { ErrorLine } from "@/components/common"
import { EmailHtml, knownHeight, placedContentIds, useDark } from "@/components/common/EmailHtml"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { useMessageEmail } from "@/features/conversation/queries"
import { useNear } from "@/hooks/use-near"
import { attachmentUrl, type Message } from "@/lib/api"
import { useEmailView } from "@/lib/emailView"
import { useSession } from "@/lib/session"

const ORIGINAL_HEIGHT = 240

function isInlineImage(a: Message["attachments"][number]) {
  return !!a.content_id && a.content_type.startsWith("image/") && a.content_type !== "image/svg+xml"
}

export function useMail(m: Message, expandQuoted: boolean) {
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
  const full = quotedShown || originalHtml ? detail.data : undefined
  const html = originalHtml ?? (full ? full.full_html : m.html)
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
  return {
    m,
    slot,
    dark,
    waiting,
    originalHtml,
    canOriginal,
    quotedShown,
    html,
    text,
    remote,
    images,
    asSent,
    inline,
    listed,
    heightKey: `${m.id}:original`,
    loadingQuoted: quotedShown && !waiting && detail.isPending,
    error: (quotedShown || wantOriginal) && detail.error,
    showImages: () => setImages(true),
    toggleAsSent: () => setAsSent((x) => !x),
    toggleQuoted: () => setQuotedOwn(!quotedShown),
    toggleOriginal: () => {
      setOriginalOwn(!originalHtml)
      if (!originalHtml && detail.error) void detail.refetch()
    },
  }
}

export type Mail = ReturnType<typeof useMail>

export function mailSurface(mail: Mail) {
  if (mail.waiting || mail.originalHtml) return mail.dark && !mail.asSent ? "original-dark" : "original-light"
  return mail.html ? "reading" : "text"
}

export function MailBody({ mail }: { mail: Mail }) {
  const { waiting, originalHtml, html, text, images, inline } = mail
  return (
    <>
      {waiting ? (
        <div ref={mail.slot} className="w-full" data-testid="original-loading">
          <Skeleton className="w-full rounded-none" style={{ height: knownHeight(mail.heightKey) ?? ORIGINAL_HEIGHT }} />
        </div>
      ) : originalHtml ? (
        <EmailHtml
          html={originalHtml}
          images={images}
          inline={inline}
          original
          asSent={mail.asSent}
          heightKey={mail.heightKey}
          initialHeight={ORIGINAL_HEIGHT}
        />
      ) : html ? (
        <EmailHtml html={html} images={images} inline={inline} />
      ) : (
        text && <div className="text-reading break-words whitespace-pre-wrap">{text}</div>
      )}
      {mail.loadingQuoted && <Skeleton className="mt-2 h-10 w-full" />}
    </>
  )
}

const control = "h-6 px-1.5 text-caption font-normal text-muted-foreground hover:text-foreground"

export function MailControls({ mail }: { mail: Mail }) {
  const { t } = useLingui()
  const { m, waiting, originalHtml, canOriginal, quotedShown, remote, images, dark, asSent } = mail
  return (
    <>
      <ErrorLine error={mail.error} className="basis-full text-caption" />
      {!waiting && canOriginal && (
        <Button variant="ghost" size="sm" className={control} onClick={mail.toggleOriginal} aria-pressed={!!originalHtml} data-testid="original-toggle">
          {originalHtml ? <Trans>Show reading view</Trans> : <Trans>Show original</Trans>}
        </Button>
      )}
      {!waiting && originalHtml && dark && (
        <Button
          variant="ghost"
          size="sm"
          className={control}
          onClick={mail.toggleAsSent}
          aria-pressed={asSent}
          title={asSent ? t`Darken the colours to match the theme` : t`Show the colours the sender chose, on white`}
          data-testid="as-sent-toggle"
        >
          {asSent ? <Trans>Darken</Trans> : <Trans>Show as sent</Trans>}
        </Button>
      )}
      {!waiting && m.email?.quoted && !originalHtml && (
        <Button variant="ghost" size="sm" className={control} onClick={mail.toggleQuoted} aria-expanded={quotedShown} data-testid="quoted-toggle">
          {quotedShown ? <Trans>Hide quoted text</Trans> : <Trans>Show quoted text</Trans>}
        </Button>
      )}
      {!waiting && remote && !images && (
        <Button
          variant="ghost"
          size="sm"
          className={control}
          onClick={mail.showImages}
          title={t`Remote images can tell the sender that you opened the mail.`}
          data-testid="load-images"
        >
          <Trans>Load images</Trans>
        </Button>
      )}
    </>
  )
}
