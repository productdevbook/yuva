import { existsSync, readdirSync, readFileSync, statSync } from "node:fs"
import { join } from "node:path"

const dist = join(import.meta.dirname, "../dist")
const repo = join(import.meta.dirname, "../..")
const REPO = "https://github.com/productdevbook/yuva/blob/main/"

function* walk(dir: string): Generator<string> {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name)
    if (statSync(p).isDirectory()) yield* walk(p)
    else if (p.endsWith(".html")) yield p
  }
}

function resolves(path: string) {
  const clean = decodeURIComponent(path)
  const candidates = clean.endsWith("/") ? [`${clean}index.html`] : [clean, `${clean}.html`, `${clean}/index.html`]
  return candidates.some((c) => existsSync(join(dist, c)) && statSync(join(dist, c)).isFile())
}

const ids = new Map<string, Set<string>>()
function idsOf(file: string) {
  if (!ids.has(file)) ids.set(file, new Set([...readFileSync(file, "utf8").matchAll(/\sid="([^"]+)"/g)].map((m) => m[1]!)))
  return ids.get(file)!
}

const slug = (heading: string) =>
  heading.toLowerCase().replace(/[`*_]/g, "").replace(/[^\p{L}\p{N}\s-]/gu, "").trim().replace(/\s/g, "-")

function repoAnchor(file: string, hash: string) {
  const headings = [...readFileSync(file, "utf8").matchAll(/^#{1,6}\s+(.+)$/gm)].map((m) => slug(m[1]!))
  return headings.includes(hash)
}

let checked = 0
const broken: string[] = []
for (const file of walk(dist)) {
  const html = readFileSync(file, "utf8")
  const page = file.slice(dist.length)
  for (const m of html.matchAll(/\s(?:href|src|srcset)="([^"]+)"/g)) {
    for (const raw of m[1]!.split(",").map((s) => s.trim().split(/\s+/)[0]!)) {
      const [target, hash] = raw.split("#") as [string, string | undefined]
      if (raw.startsWith(REPO)) {
        checked++
        const local = join(repo, target.slice(REPO.length))
        if (!existsSync(local)) broken.push(`${page} -> ${raw} (not in the repository)`)
        else if (hash && local.endsWith(".md") && !repoAnchor(local, hash)) broken.push(`${page} -> ${raw} (no heading #${hash})`)
        continue
      }
      if (raw.startsWith("#")) {
        checked++
        if (!idsOf(file).has(raw.slice(1))) broken.push(`${page} -> ${raw}`)
        continue
      }
      if (!raw.startsWith("/") || raw.startsWith("//")) continue
      const path = target.split("?")[0]!
      checked++
      if (!resolves(path || "/")) broken.push(`${page} -> ${raw}`)
      else if (hash) {
        const t = path.endsWith("/") ? join(dist, path, "index.html") : join(dist, path)
        if (!idsOf(t).has(hash)) broken.push(`${page} -> ${raw} (no #${hash})`)
      }
    }
  }
}
console.log(`links checked: ${checked}, broken: ${broken.length}`)
for (const b of broken) console.log(`  ${b}`)
process.exit(broken.length ? 1 : 0)
