import { msg } from "@lingui/core/macro"
import { ApiError } from "../client/api"
import { canCapture, captureScreen } from "../panel/capture"
import { el, fileSize, iconButton, textButton } from "../panel/helpers"
import { categoryIcon, icons } from "../panel/icons"
import type { AppContext } from "../types"
import type { Translate } from "./apps"

const maxFiles = 10

const prompts = {
  bug: msg`What happened, and what did you expect?`,
  idea: msg`What would make this better for you?`,
  praise: msg`What do you like?`,
  other: msg`What's on your mind?`,
}

function currentPage(): { page_url?: string; page_title?: string } {
  if (!/^https?:$/.test(location.protocol)) return {}
  const page_url = location.origin + location.pathname
  if (page_url.length > 2000) return {}
  const page_title = Array.from(document.title.trim()).slice(0, 300).join("").trim()
  return page_title ? { page_url, page_title } : { page_url }
}

export function feedbackApp(context: AppContext, t: Translate, root: HTMLElement): () => void {
  const categories = context.inbox.feedback_category_items
  let category = categories[0]?.key ?? "other"
  let score = 0
  let files: File[] = []
  let phase: "form" | "sending" | "sent" = "form"
  let error = ""
  let created = ""
  let destroyed = false

  const text = el("textarea", "field")
  text.rows = 4
  text.dir = "auto"
  text.maxLength = 10000
  text.addEventListener("input", () => syncSend())
  const picker = el("input")
  picker.type = "file"
  picker.multiple = true
  picker.hidden = true
  picker.addEventListener("change", () => {
    add(Array.from(picker.files ?? []))
    picker.value = ""
  })
  const send = el("button", "primary")
  send.type = "button"
  send.addEventListener("click", () => void submit())

  function add(more: File[]): void {
    files = [...files, ...more].slice(0, maxFiles)
    render()
  }

  function syncSend(): void {
    send.disabled = phase === "sending" || (text.value.trim() === "" && files.length === 0)
  }

  async function submit(): Promise<void> {
    const body = text.value.trim()
    if (phase === "sending" || (!body && files.length === 0)) return
    phase = "sending"
    error = ""
    render()
    try {
      const result = await context.client.sendFeedback({
        category,
        body: body || undefined,
        score: score || undefined,
        allow_email: false,
        files,
        ...currentPage(),
      })
      context.created(result)
      created = result.conversation.id
      phase = "sent"
    } catch (failure) {
      phase = "form"
      const status = failure instanceof ApiError ? failure.status : 0
      error =
        status === 413
          ? t(msg`The file is too large.`)
          : status === 415
            ? t(msg`This file type is not accepted.`)
            : status === 429
              ? t(msg`Too many attempts. Wait a moment and try again.`)
              : failure instanceof ApiError && failure.code === "network"
                ? t(msg`You seem to be offline.`)
                : t(msg`Could not send. Try again.`)
    }
    if (!destroyed) render()
  }

  function section(label: string): HTMLElement {
    const sec = el("section", "sec")
    sec.append(el("h2", "label", label))
    return sec
  }

  function render(): void {
    if (destroyed) return
    if (phase === "sent") {
      const card = el("div", "done-card")
      card.insertAdjacentHTML("beforeend", icons.check)
      const actions = el("div", "rowbtns")
      actions.append(
        textButton(t(msg`Done`), "quiet", () => context.back()),
        textButton(t(msg`View feedback`), "primary", () => context.viewConversation(created))
      )
      card.append(
        el("p", "empty", t(msg`Thanks for your feedback`)),
        el("p", "hint", t(msg`The team reads every note. Replies show up in your messages.`)),
        actions
      )
      const sec = el("section", "sec")
      sec.append(card)
      root.replaceChildren(sec)
      return
    }
    const kinds = section(`01 — ${t(msg`What is it about`)}`)
    const group = el("div", "seg")
    group.setAttribute("role", "group")
    group.setAttribute("aria-label", t(msg`What is it about`))
    for (const item of categories) {
      const button = textButton("", "", () => {
        category = item.key
        render()
      })
      button.insertAdjacentHTML("beforeend", categoryIcon(item.icon))
      button.append(el("span", "", item.name))
      button.setAttribute("aria-pressed", String(item.key === category))
      group.append(button)
    }
    kinds.append(group)

    const rate = section(`02 — ${t(msg`How is it working for you`)}`)
    const scores = el("div", "scores")
    scores.setAttribute("role", "group")
    scores.setAttribute("aria-label", t(msg`How is it working for you`))
    for (let n = 1; n <= 5; n++) {
      const button = textButton(String(n), "", () => {
        score = score === n ? 0 : n
        render()
      })
      button.setAttribute("aria-label", t(msg`${n} out of 5`))
      button.setAttribute("aria-pressed", String(n === score))
      scores.append(button)
    }
    const ends = el("div", "ends")
    ends.append(el("span", "", t(msg`Not good`)), el("span", "", t(msg`Great`)))
    rate.append(scores, ends)

    const more = section(`03 — ${t(msg`Tell us more`)}`)
    text.placeholder = t(Object.hasOwn(prompts, category) ? prompts[category as keyof typeof prompts] : prompts.other)
    text.setAttribute("aria-label", t(msg`Tell us more`))
    text.disabled = phase === "sending"
    const attach = el("div", "attach")
    const chips = el("div", "chips")
    chips.hidden = files.length === 0
    files.forEach((file, index) => {
      const chip = el("span", "chip")
      chip.insertAdjacentHTML("beforeend", icons.file)
      const two = el("span", "two")
      two.append(el("b", "", file.name || "image"), el("span", "", fileSize(context.locale, file.size)))
      chip.append(
        two,
        iconButton(
          icons.closeSmall,
          t(msg`Remove ${file.name}`),
          () => {
            files = files.filter((_, i) => i !== index)
            render()
          },
          "icon sm"
        )
      )
      chips.append(chip)
    })
    const attachButton = iconButton(icons.attach, t(msg`Attach files`), () => picker.click(), "icon tool")
    attach.append(chips, attachButton)
    if (canCapture()) {
      attach.append(
        iconButton(
          icons.shot,
          t(msg`Take a screenshot`),
          () =>
            void captureScreen(root.closest<HTMLElement>(".panel") ?? undefined).then((file) => file && add([file])),
          "icon tool"
        )
      )
    }
    attach.append(picker)
    more.append(text, attach)

    const foot = el("section", "sec")
    send.textContent = phase === "sending" ? t(msg`Sending…`) : t(msg`Send feedback`)
    foot.append(send)
    if (error) foot.append(el("p", "error", error))
    foot.append(el("p", "note", t(msg`The team reads every note. Replies show up in your messages.`)))
    syncSend()
    root.replaceChildren(kinds, rate, more, el("div", "fill"), foot)
  }

  render()
  return () => {
    destroyed = true
  }
}
