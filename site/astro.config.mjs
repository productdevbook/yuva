// @ts-check
import react from "@astrojs/react"
import { lingui, linguiTransformerBabelPreset } from "@lingui/vite-plugin"
import babel from "@rolldown/plugin-babel"
import tailwindcss from "@tailwindcss/vite"
import { defineConfig } from "astro/config"
import { SITE } from "./src/lib/routes"

export default defineConfig({
  site: SITE,
  output: "static",
  integrations: [react()],
  i18n: {
    locales: ["en", "tr"],
    defaultLocale: "en",
    routing: { prefixDefaultLocale: false },
  },
  build: { format: "directory", assets: "_site", inlineStylesheets: "always" },
  devToolbar: { enabled: false },
  markdown: { syntaxHighlight: false },
  vite: {
    plugins: [
      tailwindcss(),
      lingui(),
      babel({
        include: [/\.(?:[jt]sx?|[cm][jt]s|astro)(?:$|\?)/],
        presets: [linguiTransformerBabelPreset()],
      }),
    ],
    ssr: { external: ["@resvg/resvg-js"] },
  },
})
