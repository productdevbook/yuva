import { useEffect, useRef } from "react"

type HotkeyMap = Record<string, (e: KeyboardEvent) => void>

const PREFIX = "g"
const SEQUENCE_MS = 1500

function isTyping(target: EventTarget | null) {
  if (!(target instanceof HTMLElement)) return false
  return target.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName)
}

function hasOpenLayer() {
  return !!document.querySelector(
    ':is([role="dialog"], [role="alertdialog"], [role="menu"], [role="listbox"]):not([data-closed])',
  )
}

let last: { key: string; at: number } | null = null
const sequences = new WeakMap<KeyboardEvent, string>()

if (typeof window !== "undefined") {
  window.addEventListener(
    "keydown",
    (e) => {
      if (["Shift", "Control", "Alt", "Meta"].includes(e.key)) return
      if (e.ctrlKey || e.metaKey || e.altKey || isTyping(e.target)) {
        last = null
        return
      }
      const chained = last?.key === PREFIX && e.timeStamp - last.at < SEQUENCE_MS
      sequences.set(e, chained ? `${PREFIX} ${e.key}` : e.key)
      last = chained ? null : { key: e.key, at: e.timeStamp }
    },
    true,
  )
}

export function useHotkeys(map: HotkeyMap, enabled = true) {
  const ref = useRef(map)
  ref.current = map
  useEffect(() => {
    if (!enabled) return
    const onKey = (e: KeyboardEvent) => {
      if (e.defaultPrevented || e.ctrlKey || e.metaKey || e.altKey) return
      if (isTyping(e.target) || hasOpenLayer()) return
      const fn = ref.current[sequences.get(e) ?? e.key]
      if (!fn) return
      e.preventDefault()
      fn(e)
    }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [enabled])
}
