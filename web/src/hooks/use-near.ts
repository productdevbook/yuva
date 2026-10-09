import { useEffect, useState, type RefObject } from "react"

export function useNear(ref: RefObject<Element | null>, active: boolean, margin = "800px") {
  const [near, setNear] = useState(false)
  useEffect(() => {
    const el = ref.current
    if (!active || near || !el) return
    if (typeof IntersectionObserver === "undefined") {
      setNear(true)
      return
    }
    const io = new IntersectionObserver((entries) => entries.some((e) => e.isIntersecting) && setNear(true), { rootMargin: margin })
    io.observe(el)
    return () => io.disconnect()
  }, [ref, active, near, margin])
  return near
}
