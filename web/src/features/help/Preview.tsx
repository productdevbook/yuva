import { Trans, useLingui } from "@lingui/react/macro"
import { LoaderIcon } from "lucide-react"

import { MailFrame } from "@/components/common/MailFrame"
import { ErrorLine } from "@/features/help/text"
import { useRenderedMarkdown } from "@/features/help/queries"
import { useDebounced } from "@/hooks/use-debounced"
import { useTheme } from "@/lib/theme"

const escapeHtml = (s: string) =>
  s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;")

const STYLE = `
html{background:#fff}
html.dark{background:#141414}
body{margin:0;padding:24px;font:16px/1.65 ui-sans-serif,system-ui,-apple-system,"Segoe UI",sans-serif;color:#1c1917;overflow-wrap:anywhere}
html.dark body{color:#e7e5e4}
h1{font-size:28px;line-height:1.2;margin:0 0 20px}
h2{font-size:22px;margin:32px 0 8px}
h3{font-size:18px;margin:24px 0 6px}
p,ul,ol,pre,blockquote,table{margin:0 0 16px}
a{color:#2563eb}
html.dark a{color:#8ab4ff}
img{max-width:100%;height:auto;border-radius:8px}
code{font:14px ui-monospace,SFMono-Regular,Menlo,monospace;background:rgba(127,127,127,.15);padding:1px 5px;border-radius:5px}
pre{background:rgba(127,127,127,.12);padding:12px 14px;border-radius:10px;overflow:auto}
pre code{background:none;padding:0}
blockquote{border-inline-start:3px solid rgba(127,127,127,.4);padding-inline-start:14px;margin-inline-start:0;color:inherit;opacity:.85}
table{border-collapse:collapse;width:100%}
th,td{border:1px solid rgba(127,127,127,.35);padding:6px 10px;text-align:start}
`

export function PreviewPane({
  title,
  html,
  fetching,
  error,
  frameTitle,
}: {
  title: string
  html: string
  fetching: boolean
  error: unknown
  frameTitle: string
}) {
  useTheme()
  const dark = document.documentElement.classList.contains("dark")
  const doc = `<!doctype html><html${dark ? ' class="dark"' : ""}><head><meta charset="utf-8"><style>${STYLE}</style></head><body><h1>${escapeHtml(title)}</h1>${html}</body></html>`
  return (
    <div className="flex min-w-0 flex-col gap-2" data-testid="help-preview-pane">
      <div className="flex items-center gap-2 text-small text-faint">
        <Trans>Preview</Trans>
        {fetching && <LoaderIcon className="size-3 animate-spin" />}
      </div>
      <ErrorLine error={error} />
      <div className="overflow-hidden rounded-[14px] border bg-card">
        <MailFrame html={doc} title={frameTitle} dark={dark} />
      </div>
    </div>
  )
}

export function ArticlePreview({ centerId, title, body }: { centerId: string; title: string; body: string }) {
  const { t } = useLingui()
  const rendered = useRenderedMarkdown(centerId, useDebounced(body, 700))
  return (
    <PreviewPane
      title={title}
      html={rendered.data ?? ""}
      fetching={rendered.isFetching}
      error={rendered.error}
      frameTitle={t`Preview of the article`}
    />
  )
}

export function OriginalText({
  source,
  language,
  empty,
}: {
  source?: { title: string; body: string }
  language: string
  empty?: React.ReactNode
}) {
  return (
    <div className="flex min-w-0 flex-col gap-2" data-testid="help-original-pane">
      <div className="text-small text-faint">
        <Trans>Original in {language}</Trans>
      </div>
      {source ? (
        <div className="rounded-xl border bg-surface px-4 py-3">
          <h2 className="mb-2 text-title break-words" dir="auto">
            {source.title}
          </h2>
          <div className="font-mono text-[13.5px] leading-relaxed break-words whitespace-pre-wrap" dir="auto">
            {source.body}
          </div>
        </div>
      ) : (
        <p className="rounded-xl border border-dashed px-4 py-6 text-body text-faint">
          {empty ?? <Trans>The main language has no text for this article yet.</Trans>}
        </p>
      )}
    </div>
  )
}
