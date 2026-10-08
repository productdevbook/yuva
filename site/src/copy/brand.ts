import type { I18n } from "@lingui/core"
import { msg } from "@lingui/core/macro"
import { brandAssets } from "@/lib/routes"

export function brandCopy(i18n: I18n) {
  return {
    meta: {
      title: i18n._(msg`Yuva brand assets: logo, colours and name`),
      description: i18n._(msg`Download the Yuva logo as SVG or PNG, copy the brand colours, and read how the name and logo may be used.`),
    },
    kicker: i18n._(msg`Brand`),
    title: i18n._(msg`Brand assets`),
    lead: i18n._(
      msg`The Yuva logo, colours and typeface, ready for articles, integrations and app listings. Please read the rules for the name and logo before you use them.`
    ),
    copy: i18n._(msg`Copy SVG`),
    copied: i18n._(msg`Copied`),
    downloadSvg: i18n._(msg`Download SVG`),
    downloadPng: i18n._(msg`Download PNG`),
    logoTitle: i18n._(msg`Logo`),
    logos: [
      {
        title: i18n._(msg`App icon`),
        body: i18n._(msg`The mark on its rounded square. Use it wherever Yuva appears as an app or a link.`),
        src: brandAssets.icon,
        file: "yuva-icon.svg",
        dark: false,
      },
      {
        title: i18n._(msg`Full-bleed icon`),
        body: i18n._(msg`For app stores and platforms that round the corners themselves.`),
        src: brandAssets.iconSquare,
        file: "yuva-icon-square.svg",
        png: brandAssets.iconPng,
        dark: false,
      },
      {
        title: i18n._(msg`Logo with name`),
        body: i18n._(msg`For light backgrounds.`),
        src: brandAssets.logo,
        file: "yuva-logo.svg",
        dark: false,
      },
      {
        title: i18n._(msg`Logo with name, white`),
        body: i18n._(msg`For dark backgrounds.`),
        src: brandAssets.logoWhite,
        file: "yuva-logo-white.svg",
        dark: true,
      },
    ] as { title: string; body: string; src: string; file: string; png?: string; dark: boolean }[],
    colorsTitle: i18n._(msg`Colours`),
    colorsLead: i18n._(msg`Click a colour to copy its hex code.`),
    colors: [
      { name: i18n._(msg`Yuva orange`), hex: "#D4431C" },
      { name: i18n._(msg`Glow`), hex: "#FF8A5C" },
      { name: i18n._(msg`Ember`), hex: "#E4572E" },
      { name: i18n._(msg`Ink`), hex: "#0B0B0F" },
      { name: i18n._(msg`White`), hex: "#FFFFFF" },
    ],
    typeTitle: i18n._(msg`Typeface`),
    typeBody: i18n._(msg`The name and this site are set in Geist, an open-source typeface under the SIL Open Font License.`),
    rulesTitle: i18n._(msg`Using the name and logo`),
    may: i18n._(msg`You may`),
    mayItems: [
      i18n._(msg`say that your product uses, integrates with or is built on Yuva;`),
      i18n._(msg`use the name, unchanged, to refer to the unmodified project;`),
      i18n._(msg`keep the name in a private, internal deployment, modified or not.`),
    ],
    mayNot: i18n._(msg`Not without written permission`),
    mayNotItems: [
      i18n._(msg`offer a hosted or managed service, or a product, under the Yuva name or a confusingly similar one;`),
      i18n._(msg`publish a modified version under the Yuva name — rename your fork;`),
      i18n._(msg`use the logo in a way that suggests endorsement by the project.`),
    ],
    shape: i18n._(msg`Keep the logo as it is: do not change its shape, proportions or colours.`),
    trademark: i18n._(msg`Read TRADEMARK.md`),
    downloadIcon: i18n._(msg`Download the icon`),
    tabTitle: i18n._(msg`Yuva: one inbox for every product`),
    sizesTitle: i18n._(msg`Sizes`),
    sizesBody: i18n._(msg`The mark is drawn to stay readable from an app store listing down to a 16-pixel browser tab.`),
    sample: i18n._(msg`One inbox for every product you run.`),
    weights: i18n._(msg`Regular, Medium and Semibold`),
    do: i18n._(msg`Do`),
    doItems: [i18n._(msg`On light backgrounds`), i18n._(msg`On dark backgrounds`)],
    dont: i18n._(msg`Don't`),
    dontItems: [i18n._(msg`Stretch or squash it`), i18n._(msg`Change its colours`), i18n._(msg`Rotate it`), i18n._(msg`Add shadows or outlines`)],
  }
}

export type BrandCopy = ReturnType<typeof brandCopy>
