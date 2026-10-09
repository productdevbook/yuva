import { useLingui } from "@lingui/react/macro"
import { SearchIcon, XIcon } from "lucide-react"
import { useEffect, useRef } from "react"

import { Kbd } from "@/components/common"
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from "@/components/ui/input-group"
import { setListQuery, useListSearch } from "@/features/inbox/listSearch"

export function ListSearchBox() {
  const { t } = useLingui()
  const { query, focus } = useListSearch()
  const input = useRef<HTMLInputElement>(null)
  useEffect(() => {
    if (focus) input.current?.focus()
  }, [focus])
  return (
    <InputGroup className="h-10 w-[min(420px,100%)] justify-self-center rounded-[10px] border-border bg-card shadow-none dark:bg-card">
      <InputGroupAddon>
        <SearchIcon className="size-[15px] text-faint" />
      </InputGroupAddon>
      <InputGroupInput
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
        data-testid="list-search"
      />
      <InputGroupAddon align="inline-end">
        {query ? (
          <InputGroupButton size="icon-xs" onClick={() => setListQuery("")} aria-label={t`Clear the search`}>
            <XIcon />
          </InputGroupButton>
        ) : (
          <Kbd className="phone:hidden">/</Kbd>
        )}
      </InputGroupAddon>
    </InputGroup>
  )
}
