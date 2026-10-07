import { useSyncExternalStore } from "react"

type InstallPromptEvent = Event & {
  prompt: () => Promise<void>
  userChoice: Promise<{ outcome: "accepted" | "dismissed" }>
}

type State = { installPrompt: InstallPromptEvent | null; waiting: ServiceWorker | null }

let state: State = { installPrompt: null, waiting: null }
const listeners = new Set<() => void>()
let navigateTo: ((path: string) => void) | null = null
let pendingPath: string | null = null
let updateRequested = false

function set(patch: Partial<State>) {
  state = { ...state, ...patch }
  listeners.forEach((l) => l())
}

function usePwa(): State {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
    () => state,
  )
}

export function useInstallPrompt() {
  const prompt = usePwa().installPrompt
  if (!prompt) return null
  return async () => {
    await prompt.prompt()
    await prompt.userChoice
    set({ installPrompt: null })
  }
}

export function useWaitingUpdate() {
  const waiting = usePwa().waiting
  if (!waiting) return null
  return () => {
    updateRequested = true
    waiting.postMessage({ type: "skip-waiting" })
  }
}

export function onServiceWorkerNavigate(fn: ((path: string) => void) | null) {
  navigateTo = fn
  if (fn && pendingPath) {
    fn(pendingPath)
    pendingPath = null
  }
}

export function serviceWorkerSupported() {
  return "serviceWorker" in navigator && window.isSecureContext
}

export async function serviceWorkerRegistration() {
  if (!serviceWorkerSupported()) return null
  return (await navigator.serviceWorker.getRegistration("/")) ?? null
}

function watch(reg: ServiceWorkerRegistration) {
  const offer = () => {
    if (reg.waiting && navigator.serviceWorker.controller) set({ waiting: reg.waiting })
  }
  offer()
  reg.addEventListener("updatefound", () => {
    const next = reg.installing
    next?.addEventListener("statechange", () => {
      if (next.state === "installed") offer()
    })
  })
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible") void reg.update().catch(() => {})
  })
  setInterval(() => void reg.update().catch(() => {}), 60 * 60 * 1000)
}

export function startPwa() {
  window.addEventListener("beforeinstallprompt", (e) => {
    e.preventDefault()
    set({ installPrompt: e as InstallPromptEvent })
  })
  window.addEventListener("appinstalled", () => set({ installPrompt: null }))
  if (!serviceWorkerSupported() || !import.meta.env.PROD) return
  navigator.serviceWorker.addEventListener("controllerchange", () => {
    if (!updateRequested) return
    updateRequested = false
    window.location.reload()
  })
  navigator.serviceWorker.addEventListener("message", (e: MessageEvent<{ type?: string; url?: string }>) => {
    if (
      e.data?.type !== "navigate" ||
      typeof e.data.url !== "string" ||
      !e.data.url.startsWith("/") ||
      e.data.url.startsWith("//")
    )
      return
    if (navigateTo) navigateTo(e.data.url)
    else pendingPath = e.data.url
  })
  navigator.serviceWorker.startMessages()
  navigator.serviceWorker
    .register("/sw.js", { scope: "/" })
    .then(watch)
    .catch(() => {})
}
