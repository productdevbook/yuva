import { Trans, useLingui } from "@lingui/react/macro"
import { FileIcon, LinkIcon } from "lucide-react"

import { formatBytes, formatShort } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { SectionTitle } from "@/features/contact/ContactDetails"
import { useMessages } from "@/features/conversation/queries"
import { attachmentUrl } from "@/lib/api"
import { useSession } from "@/lib/session"

const URL_RE = /https?:\/\/[^\s<>"'`]+[^\s<>"'`.,;:!?)\]]/g

function isMedia(type: string) {
  return (type.startsWith("image/") && type !== "image/svg+xml") || type.startsWith("video/")
}

export function ConversationMedia({ conversationId }: { conversationId: string }) {
  const { i18n } = useLingui()
  const { workspaceId } = useSession()
  const messages = useMessages(conversationId)
  const items = (messages.data?.pages ?? []).flatMap((p) => p.items).filter((m) => !m.draft && m.kind !== "event")
  const files = items.flatMap((m) => m.attachments.map((a) => ({ a, at: m.created_at })))
  const media = files.filter((f) => isMedia(f.a.content_type))
  const docs = files.filter((f) => !isMedia(f.a.content_type))
  const seen = new Set<string>()
  const links = items.flatMap((m) =>
    [...m.body.matchAll(URL_RE)].flatMap(([url]) => {
      if (seen.has(url)) return []
      seen.add(url)
      return [{ url, at: m.created_at }]
    }),
  )
  if (messages.isPending) return <Skeleton className="h-24 rounded-xl" />
  const empty = media.length + docs.length + links.length === 0
  return (
    <div className="flex flex-col gap-5" data-testid="conversation-media">
      {empty && (
        <p className="py-6 text-center text-body text-faint">
          <Trans>No media, links or documents in this conversation yet.</Trans>
        </p>
      )}
      {media.length > 0 && (
        <section>
          <SectionTitle>
            <Trans>Media</Trans>
          </SectionTitle>
          <div className="grid grid-cols-3 gap-1.5">
            {media.map(({ a }) => (
              <a key={a.id} href={attachmentUrl(a, workspaceId)} target="_blank" rel="noreferrer" title={a.filename} className="aspect-square overflow-hidden rounded-lg border bg-surface">
                {a.content_type.startsWith("video/") ? (
                  <video src={attachmentUrl(a, workspaceId)} preload="metadata" muted className="size-full object-cover" />
                ) : (
                  <img src={attachmentUrl(a, workspaceId)} alt={a.filename} loading="lazy" className="size-full object-cover" />
                )}
              </a>
            ))}
          </div>
        </section>
      )}
      {docs.length > 0 && (
        <section>
          <SectionTitle>
            <Trans>Documents</Trans>
          </SectionTitle>
          <ul className="flex flex-col gap-1">
            {docs.map(({ a, at }) => (
              <li key={a.id}>
                <a href={attachmentUrl(a, workspaceId)} download={a.filename} className="flex items-center gap-2.5 rounded-lg px-2 py-1.5 text-body hover:bg-muted">
                  <FileIcon className="size-4 shrink-0 text-faint" />
                  <span className="min-w-0 flex-1 truncate">{a.filename}</span>
                  <span className="shrink-0 text-caption text-faint">
                    {formatBytes(a.size, i18n.locale)} · {formatShort(at, i18n.locale)}
                  </span>
                </a>
              </li>
            ))}
          </ul>
        </section>
      )}
      {links.length > 0 && (
        <section>
          <SectionTitle>
            <Trans>Links</Trans>
          </SectionTitle>
          <ul className="flex flex-col gap-1">
            {links.map(({ url, at }) => (
              <li key={url}>
                <a href={url} target="_blank" rel="noreferrer noopener" className="flex items-center gap-2.5 rounded-lg px-2 py-1.5 text-body hover:bg-muted">
                  <LinkIcon className="size-4 shrink-0 text-faint" />
                  <span className="min-w-0 flex-1 truncate text-brand">{url.replace(/^https?:\/\//, "")}</span>
                  <span className="shrink-0 text-caption text-faint">{formatShort(at, i18n.locale)}</span>
                </a>
              </li>
            ))}
          </ul>
        </section>
      )}
      {messages.hasNextPage && (
        <Button variant="ghost" size="sm" className="self-center" onClick={() => void messages.fetchNextPage()} disabled={messages.isFetchingNextPage}>
          <Trans>Look in older messages</Trans>
        </Button>
      )}
    </div>
  )
}
