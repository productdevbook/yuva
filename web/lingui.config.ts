import { defineConfig } from "@lingui/conf"
import { formatter } from "@lingui/format-po"

export default defineConfig({
  sourceLocale: "en",
  locales: ["en", "tr"],
  catalogs: [
    {
      path: "<rootDir>/src/locales/{locale}/messages",
      include: ["src"],
    },
  ],
  format: formatter({ lineNumbers: false }),
})
