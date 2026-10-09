import { useLingui } from "@lingui/react/macro"
import { SearchIcon, XIcon } from "lucide-react"
import { useEffect, useRef } from "react"

import { setListQuery, useListSearch } from "@/features/inbox/listSearch"

export function ListSearchBox() {
  const { t } = useLingui()
  const { query, focus } = useListSearch()
  const input = useRef<HTMLInputElement>(null)
  useEffect(() => {
    if (focus) input.current?.focus()
  }, [focus])
  return (
    <label className="flex w-[min(420px,100%)] min-w-0 items-center gap-2 justify-self-center rounded-[10px] border bg-card px-3 py-2 text-faint focus-within:border-input">
      <SearchIcon className="size-[15px] shrink-0" />
      <input
        ref={input}
        value={query}
        onChange={(e) => setListQuery(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Escape") {
            if (query) setListQuery("")
            else e.currentTarget.blur()
          }
        }}
        placeholder={t`Search people or messages`}
        aria-label={t`Search conversations`}
        className="min-w-0 flex-1 bg-transparent text-base text-foreground outline-none placeholder:text-faint md:text-sm"
        data-testid="list-search"
      />
      {query ? (
        <button type="button" onClick={() => setListQuery("")} aria-label={t`Clear the search`} className="text-faint hover:text-foreground">
          <XIcon className="size-3.5" />
        </button>
      ) : (
        <kbd className="rounded-[5px] border px-1 font-mono text-[10px] text-faint phone:hidden">/</kbd>
      )}
    </label>
  )
}
