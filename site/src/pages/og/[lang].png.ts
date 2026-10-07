import { msg } from "@lingui/core/macro"
import type { APIRoute, GetStaticPaths } from "astro"
import { i18nFor } from "@/i18n"
import { ogSvg, renderPng } from "@/lib/graphics"
import { locales, type Locale } from "@/lib/routes"

export const getStaticPaths: GetStaticPaths = () => locales.map((lang) => ({ params: { lang } }))

export const GET: APIRoute = ({ params }) => {
  const i18n = i18nFor(params.lang as Locale)
  const svg = ogSvg(
    i18n._(msg`One inbox for every product you run.`),
    i18n._(msg`Open-source customer messaging`),
    i18n._(msg`E-mail · live chat · in-app · self-hosted`),
  )
  return new Response(renderPng(svg, 1200), { headers: { "Content-Type": "image/png" } })
}
