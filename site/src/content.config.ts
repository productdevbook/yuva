import { defineCollection } from "astro:content"
import { glob } from "astro/loaders"
import { z } from "astro/zod"
import { defaultLocale, locales } from "./lib/routes"

const blog = defineCollection({
  loader: glob({ pattern: "**/*.md", base: "./src/content/blog" }),
  schema: z.object({
    title: z.string(),
    description: z.string(),
    date: z.coerce.date(),
    author: z.string(),
  }),
})

const docs = defineCollection({
  loader: glob({ pattern: ["*.md", ...locales.filter((l) => l !== defaultLocale).map((l) => `${l}/*.md`)], base: "../docs" }),
})

export const collections = { blog, docs }
