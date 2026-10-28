import { useLingui } from "@lingui/react/macro"

import type { BlockType } from "@/features/designs/doc"
import type { EmailDesignIssue, EmailFont, EmailSocialNetwork, EmailTemplateLocale } from "@/lib/api"

export const WARNING_CODES = new Set([
  "low_contrast",
  "html_near_clip",
  "html_clipped",
  "image_missing",
  "alt_missing",
  "merge_field_without_fallback",
  "sender_identity_missing",
  "translation_missing",
  "footer_language",
])

export function isWarning(i: EmailDesignIssue) {
  return WARNING_CODES.has(i.code)
}

export function useBlockNames(): Record<BlockType, string> {
  const { t } = useLingui()
  return {
    section: t`Section`,
    columns: t`Columns`,
    heading: t`Heading`,
    text: t`Text`,
    image: t`Image`,
    button: t`Button`,
    divider: t`Divider`,
    spacer: t`Space`,
    social: t`Social links`,
    footer: t`Footer text`,
    message: t`Message`,
  }
}

export function useCategory() {
  const { t } = useLingui()
  const names: Record<string, string> = {
    newsletter: t`Newsletter`,
    product: t`Product`,
    announcement: t`Announcement`,
    onboarding: t`Onboarding`,
    event: t`Event`,
    digest: t`Digest`,
  }
  return (c: string) => names[c] ?? c
}

export function useBlockHints(): Record<BlockType, string> {
  const { t } = useLingui()
  return {
    section: t`A group of blocks on its own background`,
    columns: t`Two or three columns that stack on phones`,
    heading: t`A title in three sizes`,
    text: t`Paragraphs, lists and links`,
    image: t`An uploaded picture, with a link if you like`,
    button: t`A link that looks like a button`,
    divider: t`A thin line between parts`,
    spacer: t`Empty room between blocks`,
    social: t`Links to your profiles`,
    footer: t`Your own lines above the required footer`,
    message: t`Where the text of a conversation message goes`,
  }
}

export function useFontNames(): Record<EmailFont, string> {
  const { t } = useLingui()
  return {
    system: t`System`,
    humanist: t`Humanist`,
    geometric: t`Geometric`,
    serif: t`Serif`,
    mono: t`Monospace`,
  }
}

export function useTemplateLocales(): { value: EmailTemplateLocale; label: string }[] {
  const { t } = useLingui()
  return [
    { value: "en", label: t`English` },
    { value: "tr", label: t`Turkish` },
    { value: "de", label: t`German` },
  ]
}

export const NETWORKS: EmailSocialNetwork[] = [
  "website",
  "x",
  "bluesky",
  "mastodon",
  "linkedin",
  "github",
  "youtube",
  "instagram",
  "facebook",
  "tiktok",
  "discord",
  "threads",
]

export function useNetworkNames(): Record<EmailSocialNetwork, string> {
  const { t } = useLingui()
  return {
    website: t`Website`,
    x: "X",
    bluesky: "Bluesky",
    mastodon: "Mastodon",
    linkedin: "LinkedIn",
    github: "GitHub",
    youtube: "YouTube",
    instagram: "Instagram",
    facebook: "Facebook",
    tiktok: "TikTok",
    discord: "Discord",
    threads: "Threads",
  }
}

export function useMergeFieldNames() {
  const { t } = useLingui()
  return {
    "contact.first_name": t`First name`,
    "contact.name": t`Full name`,
    "contact.email": t`E-mail address`,
    "inbox.name": t`Inbox name`,
    "list.name": t`List name`,
    unsubscribe_url: t`Unsubscribe link`,
    preferences_url: t`Preferences link`,
  } as Record<string, string>
}

export function useIssueText() {
  const { t } = useLingui()
  return (i: EmailDesignIssue): string => {
    switch (i.code) {
      case "low_contrast":
        return t`Some text is hard to read on its background, in the light or dark colours. Aim for 4.5:1.`
      case "html_near_clip":
        return t`The mail is over 90 KB. Gmail cuts mails off at about 102 KB.`
      case "html_clipped":
        return t`The mail is over 102 KB, so Gmail will cut it off and hide the unsubscribe link. Remove some blocks.`
      case "image_missing":
        return t`This image block has no image yet.`
      case "alt_missing":
        return t`Describe this image for people who cannot see it, or mark it as decorative.`
      case "merge_field_without_fallback":
        return t`A merge field has no fallback, so it can be empty for some people.`
      case "sender_identity_missing":
        return t`The inbox has no legal name or postal address for the footer. Add them in the inbox settings before sending.`
      case "required":
        return t`Something required is empty.`
      case "too_long":
        return t`This is too long.`
      case "too_many":
        return t`There are too many items here.`
      case "duplicate_id":
        return t`Two blocks have the same id.`
      case "nesting":
        return t`This block cannot sit inside this one.`
      case "invalid_rich_text":
        return t`The text has formatting e-mails cannot keep. Clear its formatting and try again.`
      case "invalid_url":
        return t`Links start with https://, http://, mailto: or tel:.`
      case "unknown_merge_field": {
        const field = i.detail
        return field
          ? t`There is no merge field called ${field}.`
          : t`This text uses a merge field that does not exist.`
      }
      case "merge_field_in_url":
        return t`Merge fields can only go in the part of a link after the question mark.`
      case "foreign_asset":
        return t`This uses an image or link that belongs to another workspace.`
      case "asset_not_allowed":
        return t`Pick an image from your library.`
      case "column_widths":
        return t`The column widths must add up to 100.`
      case "translation_missing":
        return t`This text is not translated yet, so the base language’s text shows.`
      case "footer_language":
        return t`The required footer of this version is in English: Yuva has no footer words in this language.`
      case "locale_required":
        return t`Name the base language before adding other languages.`
      case "invalid_locale":
        return t`A language is named twice, or the fallback language is not one of the design’s languages.`
      case "message_block_repeated":
        return t`A design can hold one message block.`
      case "message_block_missing":
        return t`This design has no message block.`
      case "merge_field_unavailable":
        return t`This design uses a merge field that conversation mail does not have (the list name or the unsubscribe and preference links).`
      case "merge_field_empty":
        return t`A merge field in this design has no fallback and is empty for this person.`
      case "translation_block":
        return t`A translation is left for a block that no longer has text.`
      default:
        return i.detail ?? t`This value is not valid.`
    }
  }
}
