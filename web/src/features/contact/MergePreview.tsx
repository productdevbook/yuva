import { Plural, Trans, useLingui } from "@lingui/react/macro"

import { Skeleton } from "@/components/ui/skeleton"
import { useContactConversationCount } from "@/features/contact/queries"
import { attrValue } from "@/features/contact/Section"
import type { Contact } from "@/lib/api"
import { useInboxes } from "@/lib/workspace"

function Part({ title, children }: { title: React.ReactNode; children: React.ReactNode }) {
  return (
    <section className="flex flex-col gap-1">
      <h3 className="text-small font-medium text-faint">{title}</h3>
      {children}
    </section>
  )
}

export function MergePreview({ source, target }: { source: Contact; target: Contact }) {
  const { t } = useLingui()
  const inboxes = useInboxes().data ?? []
  const conversations = useContactConversationCount(source.id)
  const emails = source.emails.filter((e) => !target.emails.includes(e))
  const own = target.attributes ?? {}
  const attrs = Object.entries(source.attributes ?? {})
  const none = (
    <p className="text-body text-muted-foreground">
      <Trans>None</Trans>
    </p>
  )
  const count = conversations.data?.count ?? 0
  return (
    <div className="flex min-w-0 flex-col gap-4 rounded-xl bg-surface p-4" data-testid="merge-preview">
      <Part title={<Trans>Conversations</Trans>}>
        {conversations.isPending ? (
          <Skeleton className="h-5 w-32" />
        ) : (
          <p className="text-body" data-testid="merge-conversations">
            {conversations.data?.more ? (
              <Trans>More than {count} conversations</Trans>
            ) : (
              <Plural value={count} _0="No conversations" one="# conversation" other="# conversations" />
            )}
          </p>
        )}
      </Part>
      <Part title={<Trans>E-mail addresses</Trans>}>
        {emails.length === 0 ? (
          none
        ) : (
          <ul className="flex flex-col gap-0.5 text-body">
            {emails.map((e) => (
              <li key={e} className="truncate">
                {e}
              </li>
            ))}
          </ul>
        )}
      </Part>
      <Part title={<Trans>External ids</Trans>}>
        {source.external_ids.length === 0 ? (
          none
        ) : (
          <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-0.5 text-body">
            {source.external_ids.map((x) => (
              <div key={`${x.inbox_id}:${x.external_id}`} className="contents">
                <dt className="truncate text-muted-foreground">{inboxes.find((i) => i.id === x.inbox_id)?.name ?? "?"}</dt>
                <dd className="truncate font-mono text-caption">{x.external_id}</dd>
              </div>
            ))}
          </dl>
        )}
      </Part>
      <Part title={<Trans>Attributes</Trans>}>
        {attrs.length === 0 ? (
          none
        ) : (
          <dl className="grid grid-cols-[minmax(0,auto)_minmax(0,1fr)] gap-x-4 gap-y-0.5 text-body">
            {attrs.map(([k, v]) => (
              <div key={k} className="contents">
                <dt className="truncate text-muted-foreground">{k}</dt>
                <dd className="break-words">
                  {k in own ? (
                    <span className="text-muted-foreground line-through" title={t`This contact keeps its own value`}>
                      {attrValue(v)}
                    </span>
                  ) : (
                    attrValue(v)
                  )}
                </dd>
              </div>
            ))}
          </dl>
        )}
        {attrs.some(([k]) => k in own) && (
          <p className="text-caption text-muted-foreground">
            <Trans>Struck-out values are not taken: this contact keeps its own value for those keys.</Trans>
          </p>
        )}
      </Part>
    </div>
  )
}
