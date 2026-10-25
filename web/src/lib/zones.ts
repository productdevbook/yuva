const DAY = 86_400_000

const formatters = new Map<string, Intl.DateTimeFormat>()

function formatter(key: string, locale: string | undefined, options: Intl.DateTimeFormatOptions) {
  const id = `${key}|${locale ?? ""}|${JSON.stringify(options)}`
  let f = formatters.get(id)
  if (!f) {
    f = new Intl.DateTimeFormat(locale, options)
    formatters.set(id, f)
  }
  return f
}

export function browserZone() {
  return Intl.DateTimeFormat().resolvedOptions().timeZone
}

export function validZone(zone: string) {
  try {
    formatter(zone, "en", { timeZone: zone })
    return true
  } catch {
    return false
  }
}

export function zoneCity(zone: string) {
  if (zone === "UTC" || zone === "Etc/UTC") return "UTC"
  return (zone.split("/").pop() ?? zone).replaceAll("_", " ")
}

function partsOf(zone: string, at: Date) {
  const parts = formatter(zone, "en-US", {
    timeZone: zone,
    hourCycle: "h23",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  }).formatToParts(at)
  const get = (type: string) => Number(parts.find((p) => p.type === type)?.value ?? 0)
  return { y: get("year"), m: get("month"), d: get("day"), h: get("hour") % 24, min: get("minute"), s: get("second") }
}

export function offsetMs(zone: string, at: Date) {
  const p = partsOf(zone, at)
  return Date.UTC(p.y, p.m - 1, p.d, p.h, p.min, p.s) - Math.floor(at.getTime() / 1000) * 1000
}

export function zoneOffset(zone: string, at: Date, locale: string) {
  const part = formatter(zone, locale, { timeZone: zone, timeZoneName: "shortOffset" })
    .formatToParts(at)
    .find((p) => p.type === "timeZoneName")
  return part?.value ?? ""
}

export function zoneLabel(zone: string, at: Date, locale: string) {
  return `${zoneCity(zone)} (${zoneOffset(zone, at, locale)})`
}

export function zoneLongName(zone: string, at: Date, locale: string) {
  const part = formatter(zone, locale, { timeZone: zone, timeZoneName: "longGeneric" })
    .formatToParts(at)
    .find((p) => p.type === "timeZoneName")
  return part?.value ?? ""
}

export function localToInstant(local: string, zone: string): Date | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})$/.exec(local)
  if (!m || !validZone(zone)) return null
  const wall = Date.UTC(Number(m[1]), Number(m[2]) - 1, Number(m[3]), Number(m[4]), Number(m[5]))
  if (Number.isNaN(wall)) return null
  const before = offsetMs(zone, new Date(wall - DAY))
  const after = offsetMs(zone, new Date(wall + DAY))
  const hits = [...new Set([before, after])]
    .map((o) => wall - o)
    .filter((t) => offsetMs(zone, new Date(t)) === wall - t)
    .sort((a, b) => a - b)
  return new Date(hits.length > 0 ? hits[0] : wall - before)
}

export function localParts(zone: string, at: Date) {
  const p = partsOf(zone, at)
  const pad = (n: number) => String(n).padStart(2, "0")
  return { date: `${p.y}-${pad(p.m)}-${pad(p.d)}`, time: `${pad(p.h)}:${pad(p.min)}` }
}

export function addDays(date: string, days: number) {
  const [y, m, d] = date.split("-").map(Number)
  const next = new Date(Date.UTC(y, m - 1, d + days))
  return next.toISOString().slice(0, 10)
}

export function clock(at: Date, zone: string, locale: string) {
  return formatter(zone, locale, { timeZone: zone, hour: "2-digit", minute: "2-digit" }).format(at)
}

export function wallClock(hhmm: string, locale: string) {
  const m = minutesOf(hhmm)
  return formatter("wall", locale, { timeZone: "UTC", hour: "2-digit", minute: "2-digit" }).format(
    new Date(Date.UTC(1970, 0, 1, 0, m === 1440 ? 1439 : m, m === 1440 ? 59 : 0))
  )
}

export function dayLabel(date: string, locale: string) {
  const [y, m, d] = date.split("-").map(Number)
  return formatter("day", locale, { timeZone: "UTC", dateStyle: "medium" }).format(new Date(Date.UTC(y, m - 1, d)))
}

export function utcLine(iso: string, locale: string) {
  return formatter("utc", locale, { dateStyle: "medium", timeStyle: "short", timeZone: "UTC" }).format(new Date(iso))
}

export function zonedDateTime(iso: string, zone: string, locale: string) {
  return formatter(zone, locale, { timeZone: zone, dateStyle: "medium", timeStyle: "short" }).format(new Date(iso))
}

export function minutesOf(hhmm: string) {
  const m = /^(\d{2}):(\d{2})$/.exec(hhmm)
  return m ? Number(m[1]) * 60 + Number(m[2]) : NaN
}

export function windowMinutes(start: string, end: string) {
  const s = minutesOf(start)
  const e = minutesOf(end)
  if (Number.isNaN(s) || Number.isNaN(e) || s >= 1440 || e > 1440 || s === e) return 0
  return (e - s + 1440) % 1440 || 1440
}

export function windowInstants(zone: string, date: string, start: string, end: string) {
  const endDate = end === "24:00" || minutesOf(end) <= minutesOf(start) ? addDays(date, 1) : date
  const from = localToInstant(`${date}T${start}`, zone)
  const to = localToInstant(`${endDate}T${end === "24:00" ? "00:00" : end}`, zone)
  return from && to ? { from, to } : null
}

export function nextWindowOpening(zone: string, send: Date, start: string, end: string) {
  const { date, time } = localParts(zone, send)
  const now = minutesOf(time)
  const s = minutesOf(start)
  const e = minutesOf(end)
  const inside = s < e ? now >= s && now < e : now >= s || now < e
  if (inside) return null
  return { date: now < s ? date : addDays(date, 1), time: start }
}

export function clockChange(zone: string, from: Date, to: Date) {
  let prev = offsetMs(zone, from)
  for (let t = from.getTime() + DAY; t <= to.getTime() + DAY; t += DAY) {
    const next = offsetMs(zone, new Date(t))
    if (next !== prev) {
      return localParts(zone, new Date(t - DAY / 2)).date
    }
    prev = next
  }
  return null
}
