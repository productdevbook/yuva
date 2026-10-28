import { Trans, useLingui } from "@lingui/react/macro"
import { LayoutTemplateIcon } from "lucide-react"
import { useState } from "react"

import { ErrorLine } from "@/components/common"
import { EmailHtml } from "@/components/common/EmailHtml"
import { useLanguageName } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { useMessageEmail } from "@/features/conversation/queries"
import type { Message } from "@/lib/api"

const HEIGHT = 240

export function DesignLine({ m }: { m: Message }) {
  const { t } = useLingui()
  const name = useLanguageName()
  const [open, setOpen] = useState(false)
  const detail = useMessageEmail(m.id, open)
  const design = m.design
  if (!design) return null
  const title = design.name || t`a deleted design`
  const language = design.locale ? name(design.locale) : ""
  const sent = !m.draft && design.version !== undefined
  const html = open ? detail.data?.original_html : undefined
  return (
    <div className="flex flex-col gap-1.5" data-testid="design-line">
      <div className="flex flex-wrap items-center gap-x-1.5 text-caption text-faint">
        <LayoutTemplateIcon className="size-3 shrink-0" />
        <span>
          <Trans>Design: {title}</Trans>
          {language && ` · ${language}`}
        </span>
        {sent && (
          <Button
            variant="ghost"
            size="sm"
            className="h-6 px-1.5 text-caption font-normal text-muted-foreground hover:text-foreground"
            aria-pressed={open}
            onClick={() => setOpen((x) => !x)}
            data-testid="design-as-sent"
          >
            {open ? <Trans>Hide the mail as sent</Trans> : <Trans>As sent</Trans>}
          </Button>
        )}
      </div>
      {open && <ErrorLine error={detail.error} className="text-caption" />}
      {open && !detail.error && !html && !detail.isPending && (
        <p className="text-caption text-faint">
          <Trans>The mail as sent is no longer kept.</Trans>
        </p>
      )}
      {open && detail.isPending && <Skeleton className="w-full" style={{ height: HEIGHT }} />}
      {html && (
        <div className="overflow-hidden rounded-xl border">
          <EmailHtml html={html} images original heightKey={`${m.id}:design`} initialHeight={HEIGHT} />
        </div>
      )}
    </div>
  )
}
