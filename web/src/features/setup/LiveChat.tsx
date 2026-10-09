import { useLingui } from "@lingui/react/macro"
import { useEffect, useRef } from "react"

import { cn } from "@/lib/utils"

function loadWidget() {
  if (customElements.get("yuva-chat") || document.querySelector("script[data-yuva-widget]")) return
  const script = document.createElement("script")
  script.src = "/yuva.js"
  script.dataset.yuvaWidget = ""
  document.head.append(script)
}

function forgetVisitor(publicKey: string) {
  try {
    // The widget keeps the visitor id and its session under this key.
    localStorage.removeItem(`yuva:${publicKey}`)
  } catch {
    // Storage can be blocked; then the widget kept nothing either.
  }
}

export function useForgetPreviewVisitor(publicKey: string | undefined, path: string) {
  useEffect(() => {
    if (!publicKey) return
    const forget = () => forgetVisitor(publicKey)
    window.addEventListener("pagehide", forget)
    return () => {
      window.removeEventListener("pagehide", forget)
      if (!window.location.pathname.startsWith(path)) forget()
    }
  }, [publicKey, path])
}

export function LiveChat({ publicKey, className }: { publicKey: string; className?: string }) {
  const { i18n } = useLingui()
  const host = useRef<HTMLDivElement>(null)
  const locale = i18n.locale
  useEffect(loadWidget, [])
  useEffect(() => {
    const chat = document.createElement("yuva-chat")
    chat.setAttribute("channel", publicKey)
    chat.setAttribute("server", window.location.origin)
    chat.setAttribute("layout", "embedded")
    chat.setAttribute("locale", locale)
    host.current?.replaceChildren(chat)
    return () => chat.remove()
  }, [publicKey, locale])
  return (
    <div
      ref={host}
      className={cn(
        "mx-auto h-[34rem] max-h-[calc(100svh-12rem)] w-full max-w-[24rem] overflow-hidden rounded-2xl border bg-card shadow-[0_1px_2px_rgb(15_23_42/0.04),0_24px_60px_-28px_rgb(15_23_42/0.35)]",
        className,
      )}
      data-testid="setup-live-chat"
    />
  )
}
