import { resolve } from "node:path"
import { lingui } from "@lingui/vite-plugin"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { defineConfig } from "vite"

const server = "http://127.0.0.1:8809"
const proxy = Object.fromEntries(
  ["/v1", "/client/v1", "/healthz", "/readyz", "/ingress"].map((path) => [path, { target: server, ws: true }]),
)

export default defineConfig({
  base: "/",
  plugins: [react(), lingui({ macroTransform: true }), tailwindcss()],
  resolve: {
    alias: {
      "@": resolve(import.meta.dirname, "./src"),
    },
  },
  server: { proxy },
  preview: { proxy },
})
