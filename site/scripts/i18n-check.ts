import { parse, type Token } from "@messageformat/parser"
import { formatter } from "@lingui/format-po"
import { readFile } from "node:fs/promises"
import { join } from "node:path"
import config from "../lingui.config"
import { bcp47, defaultLocale, locales } from "../src/lib/routes"

const dir = join(import.meta.dirname, "../src/locales")
const po = formatter({ lineNumbers: false })
const source = config.sourceLocale ?? defaultLocale
type Entry = { message?: string; translation?: string; context?: string; obsolete?: boolean }

async function load(locale: string) {
  const file = join(dir, locale, "messages.po")
  return (await po.parse(await readFile(file, "utf8"), {
    locale,
    sourceLocale: source,
    filename: file,
  })) as Record<string, Entry>
}

function shape(icu: string, locale: string) {
  const args = new Set<string>()
  const problems: string[] = []
  const rules = new Intl.PluralRules(locale)
  const categories = new Set(rules.resolvedOptions().pluralCategories)
  // Forms a count shown on the site can take; "other" alone covers a language whose only other form is "one".
  const used = new Set(Array.from({ length: 201 }, (_, n) => rules.select(n)))
  const walk = (tokens: Token[]) => {
    for (const t of tokens) {
      if (t.type === "argument" || t.type === "function") args.add(t.arg)
      if (t.type === "plural" || t.type === "select" || t.type === "selectordinal") {
        args.add(`${t.arg}:${t.type}`)
        if (t.type === "plural") {
          const keys = t.cases.map((c) => c.key).filter((k) => !k.startsWith("="))
          for (const k of keys)
            if (!categories.has(k as Intl.LDMLPluralRule))
              problems.push(`plural "${t.arg}" has "${k}", not a category of ${locale}`)
          const optional = used.size <= 2 && keys.includes("other")
          for (const c of used)
            if (!keys.includes(c) && !optional) problems.push(`plural "${t.arg}" lacks "${c}"`)
        }
        for (const c of t.cases) walk(c.tokens)
      }
    }
  }
  walk(parse(icu))
  return { args, problems }
}

let failed = false
const rows: string[] = []
const src = await load(source)
for (const locale of locales) {
  if (locale === source) continue
  const entries = await load(locale)
  let missing = 0
  const issues: string[] = []
  for (const [id, entry] of Object.entries(entries)) {
    if (entry.obsolete) continue
    const ref = src[id]
    if (!ref) {
      issues.push(`extra: ${JSON.stringify(entry.message ?? id)}`)
      continue
    }
    if (!entry.translation) {
      missing++
      continue
    }
    try {
      const want = shape(ref.message ?? "", bcp47[source as keyof typeof bcp47])
      const got = shape(entry.translation, bcp47[locale])
      const lost = [...want.args].filter((a) => !got.args.has(a))
      const added = [...got.args].filter((a) => !want.args.has(a))
      if (lost.length) issues.push(`lost {${lost.join("}, {")}}: ${JSON.stringify(ref.message)}`)
      if (added.length)
        issues.push(`unknown {${added.join("}, {")}}: ${JSON.stringify(ref.message)}`)
      for (const p of got.problems) issues.push(`${p}: ${JSON.stringify(ref.message)}`)
    } catch (e) {
      issues.push(`invalid ICU (${(e as Error).message}): ${JSON.stringify(entry.translation)}`)
    }
  }
  for (const id of Object.keys(src))
    if (!entries[id]) issues.push(`not in catalog: ${JSON.stringify(src[id]!.message)}`)
  const total = Object.keys(src).length
  rows.push(
    `messages ${locale.padEnd(3)} ${String(total - missing).padStart(4)}/${total} translated, ${missing} fall back to English, ${issues.length} problems`
  )
  for (const i of issues) rows.push(`    ${i}`)
  if (issues.length || missing) failed = true
}
console.log(rows.join("\n"))
console.log(failed ? "i18n check failed" : "i18n check passed")
process.exit(failed ? 1 : 0)
