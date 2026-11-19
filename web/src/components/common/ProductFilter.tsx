import { useLingui } from "@lingui/react/macro"
import { useSearchParams } from "react-router"

import { FilterMenu } from "@/components/common/ListBox"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { useInboxes } from "@/lib/workspace"

export function useProductScope() {
  const [params, setParams] = useSearchParams()
  const [global] = useInboxFilter()
  const inboxes = useInboxes().data
  const picked = params.get("inbox") ?? ""
  const chosen = inboxes?.some((i) => i.id === picked) ? picked : ""
  const set = (key: string, value: string) =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev)
        if (value) next.set(key, value)
        else next.delete(key)
        return next
      },
      { replace: true }
    )
  return { chosen, inboxId: chosen || global, global, params, set }
}

export function ProductFilter({
  value,
  onChange,
  testId,
}: {
  value: string
  onChange: (id: string) => void
  testId: string
}) {
  const { t } = useLingui()
  const [global] = useInboxFilter()
  const inboxes = useInboxes().data ?? []
  if (global || inboxes.length < 2) return null
  return (
    <FilterMenu
      value={value}
      onChange={onChange}
      options={inboxes.map((i) => ({ value: i.id, label: i.name }))}
      anyLabel={t`Every product`}
      label={t`Filter by product`}
      testId={testId}
    />
  )
}
