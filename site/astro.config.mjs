// @ts-check
import { lingui, linguiTransformerBabelPreset } from "@lingui/vite-plugin"
import babel from "@rolldown/plugin-babel"
import { defineConfig } from "astro/config"
import { SITE } from "./src/lib/routes"

export default defineConfig({
  site: SITE,
  output: "static",
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
      lingui(),
      babel({
        include: [/\.(?:[jt]sx?|[cm][jt]s|astro)(?:$|\?)/],
        presets: [linguiTransformerBabelPreset()],
      }),
    ],
    ssr: { external: ["@resvg/resvg-js"] },
  },
})
