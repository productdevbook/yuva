import type { ClientAttachment, ClientMessage, ClientConversation } from "../client/api"
import { icons } from "./icons"

export function el<K extends keyof HTMLElementTagNameMap>(
  tag: K,
  className?: string,
  text?: string
): HTMLElementTagNameMap[K] {
  const element = document.createElement(tag)
  if (className) element.className = className
  if (text !== undefined) element.textContent = text
  return element
}

export function iconButton(icon: string, label: string, onClick: () => void, className = "icon"): HTMLButtonElement {
  const button = el("button", className)
  button.type = "button"
  button.innerHTML = icon
  button.setAttribute("aria-label", label)
  button.title = label
  button.addEventListener("click", onClick)
  return button
}

export function textButton(label: string, className: string, onClick: () => void): HTMLButtonElement {
  const button = el("button", className, label)
  button.type = "button"
  button.addEventListener("click", onClick)
  return button
}

export function searchField(
  label: string,
  value: string,
  submit: (query: string) => void,
  input?: (query: string) => void
): HTMLFormElement {
  const form = el("form", "search")
  form.noValidate = true
  const field = el("input")
  field.type = "search"
  field.value = value
  field.placeholder = label
  field.enterKeyHint = "search"
  field.setAttribute("aria-label", label)
  field.maxLength = 200
  form.insertAdjacentHTML("beforeend", icons.search)
  form.append(field)
  form.addEventListener("submit", (event) => {
    event.preventDefault()
    submit(field.value.trim())
  })
  if (input) field.addEventListener("input", () => input(field.value))
  return form
}

export const time = (value: string) => Date.parse(value)
export const byCreated = (a: ClientMessage, b: ClientMessage) => time(a.created_at) - time(b.created_at)
export const byActivity = (a: ClientConversation, b: ClientConversation) =>
  time(b.last_message_at ?? b.created_at) - time(a.last_message_at ?? a.created_at)
export const authorKey = (m: ClientMessage) =>
  m.author.type === "contact" ? "contact" : `${m.author.type}:${m.author.name ?? ""}`
export const isImage = (a: ClientAttachment) => /^image\/(png|jpeg|gif|webp)$/.test(a.content_type)

export function initialsOf(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean)
  return Array.from((parts[0]?.[0] ?? "") + (parts.length > 1 ? (parts[parts.length - 1]?.[0] ?? "") : "")).join("")
}

export function fileKind(name: string, type: string): string {
  const ext = /\.([a-z0-9]{1,5})$/i.exec(name)?.[1]
  return (ext ?? type.split("/")[1] ?? "").toUpperCase()
}

export function fileSize(locale: string, bytes: number): string {
  const [value, unit] = bytes < 1_000_000 ? [bytes / 1000, "kilobyte"] : [bytes / 1_000_000, "megabyte"]
  return new Intl.NumberFormat(locale, { style: "unit", unit, unitDisplay: "narrow", maximumFractionDigits: 1 }).format(
    unit === "kilobyte" ? Math.max(1, Math.round(value)) : value
  )
}

export function limitSize(locale: string, bytes: number): string {
  const [value, unit] =
    bytes >= 1024 ** 3
      ? [bytes / 1024 ** 3, "gigabyte"]
      : bytes >= 1024 ** 2
        ? [bytes / 1024 ** 2, "megabyte"]
        : [bytes / 1024, "kilobyte"]
  return new Intl.NumberFormat(locale, { style: "unit", unit, unitDisplay: "short", maximumFractionDigits: 1 }).format(
    value
  )
}

export function dayLabel(locale: string, value: string): string {
  const date = new Date(value)
  const now = new Date()
  const start = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime()
  const diff = Math.round((start(date) - start(now)) / 86_400_000)
  if (diff === 0 || diff === -1) return new Intl.RelativeTimeFormat(locale, { numeric: "auto" }).format(diff, "day")
  return new Intl.DateTimeFormat(locale, {
    month: "short",
    day: "numeric",
    year: date.getFullYear() === now.getFullYear() ? undefined : "numeric",
  }).format(date)
}

export function sameDay(a: string, b: string): boolean {
  return new Date(a).toDateString() === new Date(b).toDateString()
}
