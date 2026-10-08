import type { I18n } from "@lingui/core"
import { msg } from "@lingui/core/macro"
import { blog, contact, docsRoot, releases, type Locale } from "@/lib/routes"

export function headerCopy(i18n: I18n, locale: Locale) {
  return {
    home: i18n._(msg`Yuva home`),
    signIn: i18n._(msg`Sign in`),
    sections: i18n._(msg`Sections`),
    menu: i18n._(msg`Menu`),
    logoMenu: i18n._(msg`Logo options`),
    copyLogo: i18n._(msg`Copy logo as SVG`),
    copyWordmark: i18n._(msg`Copy logo with name as SVG`),
    downloadLogo: i18n._(msg`Download logo`),
    brand: i18n._(msg`Brand assets`),
    copied: i18n._(msg`Copied`),
    nav: [
      { href: docsRoot(locale), label: i18n._(msg`Documentation`) },
      { href: blog[locale], label: i18n._(msg`Blog`) },
      { href: releases[locale], label: i18n._(msg`Releases`) },
      { href: `${docsRoot(locale)}roadmap/`, label: i18n._(msg`Roadmap`) },
      { href: contact[locale], label: i18n._(msg`Contact`) },
    ],
  }
}
