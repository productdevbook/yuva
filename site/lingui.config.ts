import { convertToTSX } from "@astrojs/compiler"
import { extractFromFileWithBabel, extractor } from "@lingui/cli/api"
import { defineConfig, type ExtractorType } from "@lingui/conf"
import { formatter } from "@lingui/format-po"
import { defaultLocale, locales } from "./src/lib/routes"

const astro: ExtractorType = {
  match: (filename) => filename.endsWith(".astro"),
  async extract(filename, code, onMessageExtracted, ctx) {
    const { code: tsx } = await convertToTSX(code, { filename, includeScripts: false, includeStyles: false })
    return extractFromFileWithBabel(filename, tsx, onMessageExtracted, ctx, {
      plugins: ["typescript", "jsx"],
      allowReturnOutsideFunction: true,
    })
  },
}

export default defineConfig({
  sourceLocale: defaultLocale,
  locales: [...locales],
  fallbackLocales: { default: defaultLocale },
  catalogs: [{ path: "<rootDir>/src/locales/{locale}/messages", include: ["<rootDir>/src"] }],
  format: formatter({ lineNumbers: false }),
  extractors: [extractor, astro],
})
