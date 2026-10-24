import { Trans, useLingui } from "@lingui/react/macro"
import { CircleHelpIcon } from "lucide-react"
import { useId, useState } from "react"

import { InputGroupButton } from "@/components/ui/input-group"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { useFolderLabels, useFolders } from "@/features/mail/folders"
import { cn } from "@/lib/utils"

type Suggestion = { id: string; label: string; detail: string; insert: string; pick: boolean }

const KEYS = ["from", "to", "cc", "subject", "has", "is", "in", "before", "after"] as const
type Key = (typeof KEYS)[number]

function useDetails(): Record<Key, string> {
  const { t } = useLingui()
  return {
    from: t`sender address or name`,
    to: t`recipient address`,
    cc: t`copied address`,
    subject: t`words in the subject`,
    has: t`what a message has`,
    is: t`state of the conversation`,
    in: t`a mail folder`,
    before: t`sent before a day`,
    after: t`sent on or after a day`,
  }
}

export function useOperatorSuggest(
  query: string,
  setQuery: (q: string) => void,
  input: React.RefObject<HTMLInputElement | null>
) {
  const { t } = useLingui()
  const details = useDetails()
  const folders = useFolders()
  const folderLabels = useFolderLabels()
  const listId = useId()
  const [caret, setCaret] = useState(0)
  const [focused, setFocused] = useState(false)
  const [dismissed, setDismissed] = useState(false)
  const [index, setIndex] = useState(0)

  const before = query.slice(0, caret)
  const start = Math.max(before.lastIndexOf(" "), before.lastIndexOf("\t")) + 1
  const quoted = (before.match(/"/g)?.length ?? 0) % 2 === 1
  const raw = before.slice(start)
  const neg = raw.startsWith("-") ? "-" : ""
  const token = raw.slice(neg.length)
  const colon = token.indexOf(":")

  let items: Suggestion[] = []
  if (!quoted && token) {
    if (colon < 0) {
      const word = token.toLowerCase()
      items = KEYS.filter((k) => k.startsWith(word)).map((k) => ({
        id: k,
        label: `${k}:`,
        detail: details[k],
        insert: `${neg}${k}:`,
        pick: true,
      }))
    } else {
      const key = token.slice(0, colon).toLowerCase()
      const value = token.slice(colon + 1).toLowerCase()
      const fixed: [string, string][] | null =
        key === "has"
          ? [["attachment", t`has an attachment`]]
          : key === "is"
            ? [["starred", t`starred by you`]]
            : key === "in"
              ? folders.map((f): [string, string] => [f, folderLabels[f]])
              : null
      if (fixed) {
        items = fixed
          .filter(([v]) => v.startsWith(value))
          .map(([v, d]) => ({
            id: `${key}:${v}`,
            label: `${key}:${v}`,
            detail: d,
            insert: `${neg}${key}:${v} `,
            pick: true,
          }))
      } else if ((key === "before" || key === "after") && value.length < 10) {
        items = [
          {
            id: `${key}:date`,
            label: `${key}:YYYY-MM-DD`,
            detail: t`a day as YYYY-MM-DD, in your time zone`,
            insert: "",
            pick: false,
          },
        ]
      }
    }
  }
  const open = focused && !dismissed && items.length > 0
  const active = Math.min(index, Math.max(items.length - 1, 0))

  const complete = (s: Suggestion) => {
    if (!s.pick) return
    const next = query.slice(0, start) + s.insert + query.slice(caret)
    const at = start + s.insert.length
    setQuery(next)
    setCaret(at)
    setIndex(0)
    requestAnimationFrame(() => input.current?.setSelectionRange(at, at))
  }

  return {
    open,
    items,
    active,
    listId,
    optionId: (i: number) => `${listId}-${i}`,
    complete,
    setActive: setIndex,
    inputProps: {
      role: "combobox" as const,
      "aria-autocomplete": "list" as const,
      "aria-expanded": open,
      "aria-controls": open ? listId : undefined,
      "aria-activedescendant": open ? `${listId}-${active}` : undefined,
      autoComplete: "off",
      spellCheck: false,
      onFocus: () => setFocused(true),
      onBlur: () => setFocused(false),
      onSelect: (e: React.SyntheticEvent<HTMLInputElement>) => setCaret(e.currentTarget.selectionStart ?? 0),
    },
    typed: (e: React.ChangeEvent<HTMLInputElement>) => {
      setCaret(e.target.selectionStart ?? e.target.value.length)
      setDismissed(false)
      setIndex(0)
    },
    onKeyDown: (e: React.KeyboardEvent<HTMLInputElement>) => {
      if (!open) return false
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        e.preventDefault()
        const n = items.length
        setIndex((active + (e.key === "ArrowDown" ? 1 : n - 1)) % n)
        return true
      }
      if ((e.key === "Tab" && !e.shiftKey) || e.key === "Enter") {
        const s = items[active]
        if (!s?.pick) return false
        e.preventDefault()
        complete(s)
        return true
      }
      if (e.key === "Escape") {
        e.preventDefault()
        e.stopPropagation()
        setDismissed(true)
        return true
      }
      return false
    },
  }
}

export function OperatorList({ s }: { s: ReturnType<typeof useOperatorSuggest> }) {
  if (!s.open) return null
  return (
    <ul
      id={s.listId}
      role="listbox"
      className="absolute inset-x-3 top-full z-30 mt-0.5 overflow-hidden rounded-lg border bg-popover py-1 text-popover-foreground shadow-md"
      data-testid="operator-suggestions"
    >
      {s.items.map((it, i) => (
        <li
          key={it.id}
          id={s.optionId(i)}
          role="option"
          aria-selected={i === s.active}
          aria-disabled={!it.pick || undefined}
          onMouseDown={(e) => e.preventDefault()}
          onMouseEnter={() => s.setActive(i)}
          onClick={() => s.complete(it)}
          className={cn(
            "flex min-w-0 items-baseline gap-2 px-3 py-1.5 text-body",
            it.pick && "cursor-pointer",
            i === s.active && "bg-muted"
          )}
        >
          <span className="shrink-0 font-mono text-small font-medium">{it.label}</span>
          <span className="min-w-0 truncate text-small text-muted-foreground">{it.detail}</span>
        </li>
      ))}
    </ul>
  )
}

export function OperatorHelp() {
  const { t } = useLingui()
  const details = useDetails()
  const [open, setOpen] = useState(false)
  const rows: { op: string; detail: string; example: string }[] = [
    { op: "from:", detail: details.from, example: "from:ada@example.com from:me" },
    { op: "to: cc:", detail: details.to, example: "to:me cc:@example.com" },
    { op: "subject:", detail: details.subject, example: 'subject:"quarterly invoice"' },
    { op: "has:attachment", detail: t`has an attachment`, example: "has:attachment invoice" },
    { op: "is:starred", detail: t`starred by you`, example: "-is:starred" },
    {
      op: "in:",
      detail: details.in,
      example: "in:archive from:ada",
    },
    {
      op: "after: before:",
      detail: t`a day as YYYY-MM-DD, in your time zone`,
      example: "after:2026-01-01 before:2026-02-01",
    },
  ]
  return (
    <>
      <InputGroupButton
        size="icon-xs"
        onClick={() => setOpen(true)}
        aria-label={t`Search operators`}
        title={t`Search operators`}
        data-testid="operator-help"
      >
        <CircleHelpIcon />
      </InputGroupButton>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-h-[85svh] overflow-y-auto sm:max-w-xl" data-testid="operator-help-sheet">
          <DialogHeader>
            <DialogTitle>
              <Trans>Search operators</Trans>
            </DialogTitle>
            <DialogDescription>
              <Trans>
                Add them to plain words. Put a minus before one to leave out what matches, and quotes around a value
                with spaces. Every operator must hold.
              </Trans>
            </DialogDescription>
          </DialogHeader>
          <dl className="grid gap-3">
            {rows.map((r) => (
              <div key={r.op} className="grid gap-0.5">
                <dt className="font-mono text-small font-medium">{r.op}</dt>
                <dd className="text-body text-muted-foreground">{r.detail}</dd>
                <dd>
                  <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-small break-all">{r.example}</code>
                </dd>
              </div>
            ))}
          </dl>
        </DialogContent>
      </Dialog>
    </>
  )
}
