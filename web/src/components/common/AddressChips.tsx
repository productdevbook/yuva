import { useLingui } from "@lingui/react/macro"
import { XIcon } from "lucide-react"
import { forwardRef, useId, useState } from "react"

import { cn } from "@/lib/utils"

export const MAX_ADDRESSES = 20

const ADDRESS = /^[^\s@,;<>"()]+@[^\s@,;<>"()]+\.[^\s@,;<>"()]+$/

export function parseAddresses(text: string) {
  const valid: string[] = []
  const invalid: string[] = []
  for (const part of text.split(/[\s,;]+/)) {
    const address = part.replace(/^<|>$/g, "").toLowerCase()
    if (!address) continue
    if (ADDRESS.test(address)) valid.push(address)
    else invalid.push(part)
  }
  return { valid, invalid }
}

export type AddressOption = { email: string; name?: string }

export const AddressChips = forwardRef<
  HTMLInputElement,
  {
    label: string
    value: string[]
    onChange: (next: string[]) => void
    options?: AddressOption[]
    onType?: (text: string) => void
    testId?: string
    className?: string
    max?: number
    flagged?: string[]
    below?: boolean
  }
>(function AddressChips(
  { label, value, onChange, options = [], onType, testId, className, max = MAX_ADDRESSES, flagged = [], below },
  ref
) {
  const { t } = useLingui()
  const id = useId()
  const [text, setText] = useState("")
  const [error, setError] = useState("")
  const [active, setActive] = useState(-1)
  const [open, setOpen] = useState(false)
  const full = value.length >= max
  const shown = options.filter((o) => !value.includes(o.email.toLowerCase())).slice(0, 6)
  const listed = open && shown.length > 0 && text.trim() !== ""

  const type = (next: string) => {
    setText(next)
    setActive(-1)
    setOpen(true)
    setError("")
    onType?.(next.trim())
  }
  const add = (raw: string) => {
    const { valid, invalid } = parseAddresses(raw)
    const merged = [...value]
    for (const a of valid) if (!merged.includes(a) && merged.length < max) merged.push(a)
    if (merged.length !== value.length) onChange(merged)
    if (invalid.length > 0) {
      setText(invalid.join(" "))
      setError(t`${invalid[0]} is not an e-mail address`)
      return false
    }
    if (valid.length > 0 && merged.length >= max && valid.some((a) => !merged.includes(a))) {
      setError(t`At most ${max} addresses`)
    }
    setText("")
    setActive(-1)
    onType?.("")
    return true
  }
  const commit = () => text.trim() === "" || add(text)

  return (
    <div className={cn("relative min-w-0", className)}>
      <div
        className="flex min-h-8 flex-wrap items-center gap-1 rounded-lg border bg-background px-1.5 py-1 focus-within:border-brand/45"
        data-testid={testId}
      >
        {value.map((email) => (
          <span
            key={email}
            className={cn(
              "flex max-w-full items-center gap-0.5 rounded-md bg-muted ps-2 text-caption",
              flagged.includes(email) && "bg-destructive/10 text-destructive"
            )}
            data-flagged={flagged.includes(email) || undefined}
            data-testid="address-chip"
          >
            <span className="max-w-56 truncate">{email}</span>
            <button
              type="button"
              className="inline-flex size-5 items-center justify-center rounded text-faint outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
              aria-label={t`Remove ${email}`}
              onClick={() => onChange(value.filter((x) => x !== email))}
            >
              <XIcon className="size-3" />
            </button>
          </span>
        ))}
        <input
          ref={ref}
          type="text"
          inputMode="email"
          autoComplete="off"
          autoCapitalize="off"
          spellCheck={false}
          value={text}
          disabled={full}
          role="combobox"
          aria-label={label}
          aria-expanded={listed}
          aria-controls={`${id}-list`}
          aria-activedescendant={listed && active >= 0 ? `${id}-${active}` : undefined}
          aria-invalid={!!error}
          placeholder={value.length === 0 ? t`Add an address` : ""}
          className="min-w-24 flex-1 bg-transparent px-1 text-small outline-none placeholder:text-faint"
          onChange={(e) => {
            if (/[,;\s]$/.test(e.target.value) && e.target.value.trim() !== "") {
              add(e.target.value)
              return
            }
            type(e.target.value)
          }}
          onPaste={(e) => {
            const pasted = e.clipboardData.getData("text")
            if (!/[,;\s]/.test(pasted.trim())) return
            e.preventDefault()
            add(pasted)
          }}
          onBlur={() => {
            commit()
            setOpen(false)
          }}
          onFocus={() => setOpen(true)}
          onKeyDown={(e) => {
            if (e.key === "ArrowDown" || e.key === "ArrowUp") {
              if (!listed) return
              e.preventDefault()
              const step = e.key === "ArrowDown" ? 1 : -1
              setActive((i) => (i + step + shown.length) % shown.length)
              return
            }
            if (e.key === "Enter" && !e.metaKey && !e.ctrlKey) {
              e.preventDefault()
              if (listed && active >= 0) add(shown[active].email)
              else commit()
              return
            }
            if (e.key === "Tab" && !e.shiftKey && text.trim() !== "") {
              if (listed && active >= 0) {
                e.preventDefault()
                add(shown[active].email)
              } else if (!commit()) e.preventDefault()
              return
            }
            if (e.key === "Backspace" && text === "" && value.length > 0) {
              onChange(value.slice(0, -1))
              return
            }
            if (e.key === "Escape" && listed) {
              e.preventDefault()
              e.stopPropagation()
              setOpen(false)
            }
          }}
        />
      </div>
      {error && (
        <p role="alert" className="px-1 pt-0.5 text-caption text-destructive">
          {error}
        </p>
      )}
      {listed && (
        <ul
          id={`${id}-list`}
          role="listbox"
          aria-label={label}
          className={cn(
            "absolute start-0 z-20 max-h-56 w-[min(360px,100%)] overflow-y-auto rounded-xl border bg-popover p-1 shadow-md",
            below ? "top-full mt-1" : "bottom-full mb-1"
          )}
        >
          {shown.map((o, i) => (
            <li
              key={o.email}
              id={`${id}-${i}`}
              role="option"
              aria-selected={i === active}
              className={cn(
                "flex min-w-0 cursor-default flex-col rounded-lg px-2.5 py-1.5 text-small",
                i === active && "bg-muted"
              )}
              onMouseDown={(e) => {
                e.preventDefault()
                add(o.email)
              }}
            >
              {o.name ? (
                <>
                  <span className="truncate font-medium">{o.name}</span>
                  <span className="truncate text-caption text-faint">{o.email}</span>
                </>
              ) : (
                <span className="truncate">{o.email}</span>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
})
