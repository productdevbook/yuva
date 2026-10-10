import { createHash } from "node:crypto"
import { readdirSync, readFileSync } from "node:fs"
import { join, relative, resolve } from "node:path"
import { lingui } from "@lingui/vite-plugin"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { defineConfig, type Plugin } from "vite"

const server = "http://127.0.0.1:8809"
const proxy = Object.fromEntries(
  ["/v1", "/client/v1", "/healthz", "/readyz", "/ingress", "/r/", "/yuva.js", "/yuva-chat.js"].map((path) => [path, { target: server, ws: true }]),
)

const EMOJIBASE = "emojibase"
const emojibaseFiles = ["en/data.json", "en/messages.json"]

function emojibase(): Plugin {
  const source = (file: string) => readFileSync(resolve(import.meta.dirname, "node_modules/emojibase-data", file))
  return {
    name: "yuva-emojibase",
    configureServer(server) {
      server.middlewares.use(`/${EMOJIBASE}`, (req, res, next) => {
        const file = (req.url ?? "").split("?")[0].replace(/^\//, "")
        if (!emojibaseFiles.includes(file)) return next()
        res.setHeader("Content-Type", "application/json")
        res.end(source(file))
      })
    },
    generateBundle() {
      for (const file of emojibaseFiles) this.emitFile({ type: "asset", fileName: `${EMOJIBASE}/${file}`, source: source(file) })
    },
  }
}

function serviceWorker(): Plugin {
  let publicDir = ""
  return {
    name: "yuva-service-worker",
    apply: "build",
    enforce: "post",
    configResolved(config) {
      publicDir = config.publicDir
    },
    generateBundle: {
      order: "post",
      handler(_, bundle) {
        const hash = createHash("sha256")
        const files: string[] = []
        for (const [name, chunk] of Object.entries(bundle)) {
          if (name.endsWith(".map") || name.startsWith(`${EMOJIBASE}/`)) continue
          files.push(`/${name}`)
          hash.update(name)
          if (chunk.type === "asset") hash.update(chunk.source)
        }
        const walk = (dir: string): string[] =>
          readdirSync(dir, { withFileTypes: true }).flatMap((e) =>
            e.isDirectory() ? walk(join(dir, e.name)) : [join(dir, e.name)],
          )
        for (const file of publicDir ? walk(publicDir) : []) {
          files.push(`/${relative(publicDir, file).split("\\").join("/")}`)
          hash.update(readFileSync(file))
        }
        const source = readFileSync(resolve(import.meta.dirname, "sw.js"), "utf8")
        hash.update(source)
        files.sort()
        this.emitFile({
          type: "asset",
          fileName: "sw.js",
          source: source
            .replace("self.__YUVA_VERSION__", JSON.stringify(hash.digest("hex").slice(0, 12)))
            .replace("self.__YUVA_PRECACHE__", JSON.stringify(files)),
        })
      },
    },
  }
}

export default defineConfig({
  base: "/",
  plugins: [react(), lingui({ macroTransform: true }), tailwindcss(), emojibase(), serviceWorker()],
  resolve: {
    alias: {
      "@": resolve(import.meta.dirname, "./src"),
    },
  },
  server: { proxy },
  preview: { proxy },
})
