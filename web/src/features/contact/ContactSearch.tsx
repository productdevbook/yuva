import { Trans, useLingui } from "@lingui/react/macro"
import { SearchIcon } from "lucide-react"
import { useEffect, useState } from "react"

import { ErrorLine, PersonAvatar } from "@/components/common"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { useContactSearch } from "@/features/contact/queries"
import { contactName } from "@/features/contact/Section"
import type { Contact } from "@/lib/api"
import { Button } from "@/components/ui/button"

export function ContactSearch({ exclude, onPick }: { exclude: string; onPick: (c: Contact) => void }) {
  const { t } = useLingui()
  const [q, setQ] = useState("")
  const [query, setQuery] = useState("")
  const results = useContactSearch(query, true)
  const candidates = (results.data ?? []).filter((c) => c.id !== exclude)
  const unnamed = t`Unnamed contact`
  useEffect(() => {
    const id = setTimeout(() => setQuery(q.trim()), 250)
    return () => clearTimeout(id)
  }, [q])
  return (
    <div className="flex flex-col gap-2">
      <div className="relative">
        <SearchIcon className="pointer-events-none absolute start-3 top-1/2 size-4 -translate-y-1/2 text-faint" />
        <Input
          type="search"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder={t`Search by name, e-mail or external id`}
          aria-label={t`Search contacts`}
          className="ps-9"
          autoFocus
          data-testid="merge-search"
        />
      </div>
      <ErrorLine error={results.error} />
      <ul className="-mx-2 flex max-h-72 flex-col overflow-y-auto" data-testid="merge-results">
        {results.isPending ? (
          <li className="px-2">
            <Skeleton className="h-10 w-full" />
          </li>
        ) : candidates.length === 0 ? (
          <li className="px-2 py-3 text-sm text-muted-foreground">
            <Trans>No other contact matches.</Trans>
          </li>
        ) : (
          candidates.map((c) => {
            const name = contactName(c, unnamed)
            const detail = c.emails[0] ?? c.external_ids[0]?.external_id
            return (
              <li key={c.id}>
                <Button
                  variant="plain"
                  size="auto"
                  className="flex w-full gap-3 rounded-lg px-2 py-1.5 hover:bg-surface focus-visible:bg-surface"
                  onClick={() => onPick(c)}
                  data-testid="merge-candidate"
                >
                  <PersonAvatar name={name} className="size-7 text-[10px]" />
                  <span className="flex min-w-0 flex-col">
                    <span className="truncate font-medium">{name}</span>
                    {detail && detail !== name && <span className="truncate text-xs text-muted-foreground">{detail}</span>}
                  </span>
                </Button>
              </li>
            )
          })
        )}
      </ul>
    </div>
  )
}
