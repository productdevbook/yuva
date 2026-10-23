import type { MessageDescriptor } from "@lingui/core"
import { msg, plural } from "@lingui/core/macro"
import type { ClientConversation, ClientInbox } from "../client/api"
import type { AppKind, AppParams } from "../types"
import { el, searchField, textButton } from "./helpers"
import { icons } from "./icons"

export interface HomeData {
  inbox: ClientInbox
  conversations: ClientConversation[]
  mailingLists: boolean
  updates: number
  locale: string
  t(descriptor: MessageDescriptor): string
  notice: HTMLElement
  openConversation(id: string): void
  allConversations(): void
  newConversation(): void
  openApp(kind: AppKind, params?: AppParams): void
}

const stateText = {
  operational: msg`All systems operational`,
  degraded: msg`Degraded performance`,
  partial_outage: msg`Partial outage`,
  major_outage: msg`Major outage`,
  maintenance: msg`Maintenance`,
}

const tone = {
  operational: "",
  degraded: " warn",
  partial_outage: " bad",
  major_outage: " bad",
  maintenance: " maint",
}

export function appTitle(kind: AppKind): MessageDescriptor {
  return kind === "feedback"
    ? msg`Feedback`
    : kind === "status"
      ? msg`Status`
      : kind === "help"
        ? msg`Help center`
        : kind === "roadmap"
          ? msg`Roadmap`
          : kind === "whats-new"
            ? msg`What's new`
            : msg`Get updates`
}

function section(label: string): HTMLElement {
  const section = el("section", "sec")
  section.append(el("h2", "label", label))
  return section
}

export function buildHome(data: HomeData): HTMLElement {
  const { inbox, t, locale } = data
  const page = el("div", "page home")

  const hero = el("section", "sec hero")
  const members = inbox.mode === "live" && inbox.open_now && inbox.presence?.available ? inbox.presence.members : []
  if (members.length > 0) {
    const avatars = el("div", "avatars")
    for (const member of members.slice(0, 3)) {
      const avatar = el("span", "avatar", member.initials || "•")
      avatar.title = member.name
      avatars.append(avatar)
    }
    hero.append(avatars)
  }
  hero.append(el("h1", "", t(msg`How can we help?`)))
  if (inbox.help) hero.append(searchField(t(msg`Search for help`), "", (query) => data.openApp("help", { query })))
  page.append(data.notice, hero)

  const latest = data.conversations[0]
  const talk = section(`01 — ${latest ? t(msg`Continue`) : t(msg`Talk to us`)}`)
  if (latest) {
    const item = el("button", latest.unread ? "item unread" : "item")
    item.type = "button"
    const at = new Date(latest.last_message_at ?? latest.created_at)
    const main = el("div", "item-main")
    const top = el("div", "item-top")
    top.append(el("span", "item-subject", latest.subject || inbox.name || t(msg`Conversation`)))
    top.append(
      el(
        "span",
        "item-time",
        new Intl.DateTimeFormat(
          locale,
          at.toDateString() === new Date().toDateString() ? { timeStyle: "short" } : { month: "short", day: "numeric" }
        ).format(at)
      )
    )
    const preview = latest.last_message
      ? `${latest.last_message.author_type === "contact" ? `${t(msg`You:`)} ` : ""}${latest.last_message.redacted ? t(msg`This message was removed`) : latest.last_message.text || t(msg`Attachment`)}`
      : ""
    const line = el("span", "item-preview", preview.replace(/\*\*|`/g, ""))
    line.dir = "auto"
    main.append(top, line)
    const mark = el("span", "avatar lg", (inbox.name || "").slice(0, 2))
    item.append(mark, main)
    if (latest.unread) item.append(el("span", "unread-dot"))
    item.addEventListener("click", () => data.openConversation(latest.id))
    talk.append(item)
    if (data.conversations.length > 1)
      talk.append(textButton(t(msg`All conversations`), "link brand", () => data.allConversations()))
  }
  const send = el("button", "primary")
  send.type = "button"
  send.append(t(msg`Send us a message`))
  send.insertAdjacentHTML("beforeend", icons.arrow)
  send.addEventListener("click", () => data.newConversation())
  talk.append(send)
  page.append(talk)

  const tiles: { app: AppKind; icon: string; title: string; text: string; count?: number }[] = []
  if (inbox.help)
    tiles.push({ app: "help", icon: icons.book, title: t(msg`Help center`), text: t(msg`Guides and answers`) })
  if (inbox.feedback_category_items.length > 0)
    tiles.push({
      app: "feedback",
      icon: icons.bubble,
      title: t(msg`Feedback`),
      text: t(msg`A bug, an idea, a thank-you`),
    })
  if (inbox.roadmap)
    tiles.push({ app: "roadmap", icon: icons.vote, title: t(msg`Roadmap`), text: t(msg`Vote on what comes next`) })
  if (inbox.whats_new)
    tiles.push({
      app: "whats-new",
      icon: icons.sparkle,
      title: t(msg`What's new`),
      text: t(msg`Changes and news`),
      count: data.updates,
    })
  if (data.mailingLists)
    tiles.push({ app: "subscribe", icon: icons.bell, title: t(msg`Get updates`), text: t(msg`News by e-mail`) })
  const status = inbox.status
  if (tiles.length > 0 || status) {
    const more = section(`02 — ${t(msg`More from the team`)}`)
    if (tiles.length > 0) {
      const grid = el("div", "tiles")
      for (const tile of tiles) {
        const button = el("button", "tile")
        button.type = "button"
        if (tile.count) {
          const top = el("span", "tile-top")
          top.insertAdjacentHTML("beforeend", tile.icon)
          const count = tile.count
          const badge = el("span", "tile-count", count > 9 ? "9+" : String(count))
          badge.setAttribute("aria-label", t(msg`${plural(count, { one: "# update", other: "# updates" })}`))
          top.append(badge)
          button.append(top)
        } else button.insertAdjacentHTML("beforeend", tile.icon)
        button.append(el("b", "", tile.title), el("span", "", tile.text))
        button.addEventListener("click", () => data.openApp(tile.app))
        grid.append(button)
      }
      more.append(grid)
    }
    if (status) {
      const row = el("button", "statusrow")
      row.type = "button"
      row.append(
        el("span", `tone${tone[status.state]}`),
        el("span", "grow", t(stateText[status.state])),
        el("span", "label", t(msg`Status`))
      )
      row.insertAdjacentHTML("beforeend", icons.chevron)
      row.addEventListener("click", () => data.openApp("status"))
      more.append(row)
    }
    page.append(more)
  }

  page.append(el("div", "fill"))
  const brand = "Yuva"
  const foot = el("section", "sec foot", t(msg`Powered by ${brand}`))
  page.append(foot)
  return page
}
