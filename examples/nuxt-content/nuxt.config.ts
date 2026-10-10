export default defineNuxtConfig({
  compatibilityDate: "2026-10-01",
  modules: ["@nuxt/content"],
  content: {
    experimental: { sqliteConnector: "native" },
  },
  vue: {
    compilerOptions: {
      isCustomElement: (tag) => tag.startsWith("yuva-"),
    },
  },
  runtimeConfig: {
    public: {
      yuvaChannel: "",
      yuvaServer: "",
    },
  },
});
