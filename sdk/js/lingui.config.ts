import { defineConfig } from "@lingui/conf";
import { formatter } from "@lingui/format-po";
import { createSwcExtractor } from "@lingui/native-tools";

export default defineConfig({
  sourceLocale: "en",
  locales: ["en", "tr"],
  catalogs: [{ path: "<rootDir>/src/locales/{locale}/messages", include: ["<rootDir>/src"] }],
  format: formatter({ lineNumbers: false }),
  extractors: [createSwcExtractor()],
});
