import { useState } from "react"

const KEY = "contact-panel"

function stored() {
  try {
    return localStorage.getItem(KEY) !== "hidden"
  } catch {
    return true
  }
}

export function usePanePreference() {
  const [open, setOpen] = useState(stored)
  const toggle = () =>
    setOpen((o) => {
      try {
        localStorage.setItem(KEY, o ? "hidden" : "shown")
      } catch {
        // Storage can be blocked; the choice then lasts for this page only.
      }
      return !o
    })
  return [open, toggle] as const
}
