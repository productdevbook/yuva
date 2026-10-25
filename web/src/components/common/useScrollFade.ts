import { useCallback, useEffect, useRef, useState } from "react"

export function useScrollFade<T extends HTMLElement>(...deps: unknown[]) {
  const ref = useRef<T>(null)
  const [fade, setFade] = useState({ left: false, right: false })
  const measure = useCallback(() => {
    const el = ref.current
    if (!el) return
    const away = Math.abs(el.scrollLeft)
    const more = el.scrollWidth - el.clientWidth - away > 1
    const rtl = getComputedStyle(el).direction === "rtl"
    const next = rtl ? { left: more, right: away > 1 } : { left: away > 1, right: more }
    setFade((f) => (f.left === next.left && f.right === next.right ? f : next))
  }, [])
  useEffect(() => {
    const el = ref.current
    if (!el) return
    el.querySelector<HTMLElement>("[aria-pressed=true]")?.scrollIntoView({ inline: "nearest", block: "nearest" })
    measure()
    const watch = new ResizeObserver(measure)
    watch.observe(el)
    return () => watch.disconnect()
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, [measure, ...deps])
  const edge = "1.5rem"
  const mask =
    fade.left || fade.right
      ? `linear-gradient(to right, ${fade.left ? "transparent" : "#000"}, #000 ${edge}, #000 calc(100% - ${edge}), ${fade.right ? "transparent" : "#000"})`
      : undefined
  return { ref, onScroll: measure, style: { maskImage: mask, WebkitMaskImage: mask } }
}
