import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useEffect } from "react"

import { api, ApiError, unwrap } from "@/lib/api"
import { serviceWorkerRegistration, serviceWorkerSupported } from "@/lib/pwa"

export const pushKeys = {
  vapid: ["push", "vapid"] as const,
  subscriptions: ["me", "push-subscriptions"] as const,
  browser: ["push", "browser"] as const,
}

type PushSupport = "supported" | "unsupported" | "needs-install"

export function pushSupport(): PushSupport {
  if (serviceWorkerSupported() && "PushManager" in window && "Notification" in window) return "supported"
  const ios =
    /iPad|iPhone|iPod/.test(navigator.userAgent) || (navigator.platform === "MacIntel" && navigator.maxTouchPoints > 1)
  if (ios && !window.matchMedia("(display-mode: standalone)").matches) return "needs-install"
  return "unsupported"
}

function deviceLabel(): string {
  const ua = navigator.userAgent
  const browser = /Edg\//.test(ua)
    ? "Edge"
    : /OPR\//.test(ua)
      ? "Opera"
      : /Firefox\//.test(ua)
        ? "Firefox"
        : /SamsungBrowser\//.test(ua)
          ? "Samsung Internet"
          : /Chrome\//.test(ua)
            ? "Chrome"
            : /Safari\//.test(ua)
              ? "Safari"
              : "Browser"
  const os = /Android/.test(ua)
    ? "Android"
    : /iPhone|iPod/.test(ua)
      ? "iPhone"
      : /iPad/.test(ua) || (/Macintosh/.test(ua) && navigator.maxTouchPoints > 1)
        ? "iPad"
        : /Mac OS X/.test(ua)
          ? "macOS"
          : /Windows/.test(ua)
            ? "Windows"
            : /CrOS/.test(ua)
              ? "ChromeOS"
              : /Linux/.test(ua)
                ? "Linux"
                : ""
  return os ? `${browser} · ${os}` : browser
}

function keyBytes(base64url: string) {
  const b64 = base64url
    .replace(/-/g, "+")
    .replace(/_/g, "/")
    .padEnd(Math.ceil(base64url.length / 4) * 4, "=")
  return Uint8Array.from(atob(b64), (c) => c.charCodeAt(0))
}

async function vapidKey() {
  try {
    return (await unwrap(api.GET("/v1/push/vapid-public-key"))).public_key
  } catch (err) {
    if (err instanceof ApiError && err.status === 404) return null
    throw err
  }
}

export function useVapidKey() {
  return useQuery({
    queryKey: pushKeys.vapid,
    queryFn: vapidKey,
    staleTime: Infinity,
    enabled: pushSupport() === "supported",
  })
}

async function browserSubscription() {
  const reg = await serviceWorkerRegistration()
  return (await reg?.pushManager.getSubscription()) ?? null
}

export function useBrowserSubscription() {
  return useQuery({
    queryKey: pushKeys.browser,
    queryFn: async () => (await browserSubscription())?.endpoint ?? null,
    enabled: pushSupport() === "supported",
  })
}

async function register(sub: PushSubscription) {
  const json = sub.toJSON()
  return unwrap(
    api.POST("/v1/me/push-subscriptions", {
      body: {
        endpoint: sub.endpoint,
        keys: { p256dh: json.keys?.p256dh ?? "", auth: json.keys?.auth ?? "" },
        user_agent: deviceLabel(),
      },
    }),
  )
}

async function ensureSubscription(vapidKey: string) {
  const reg = (await serviceWorkerRegistration()) ?? (await navigator.serviceWorker.ready)
  const key = keyBytes(vapidKey)
  let sub = await reg.pushManager.getSubscription()
  const current = sub?.options.applicationServerKey
  if (sub && current && !sameKey(new Uint8Array(current), key)) {
    await sub.unsubscribe()
    sub = null
  }
  sub ??= await reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: key })
  return register(sub)
}

export async function subscribe(vapidKey: string) {
  if ((await Notification.requestPermission()) !== "granted") return "denied" as const
  await ensureSubscription(vapidKey)
  return "subscribed" as const
}

function sameKey(a: Uint8Array, b: Uint8Array) {
  return a.length === b.length && a.every((x, i) => x === b[i])
}

export async function unsubscribe(id: string | undefined) {
  if (id)
    await unwrap(
      api.DELETE("/v1/me/push-subscriptions/{pushSubscriptionId}", { params: { path: { pushSubscriptionId: id } } }),
    )
  await (await browserSubscription())?.unsubscribe()
}

export function useRegisterPushOnStart() {
  const qc = useQueryClient()
  useEffect(() => {
    if (pushSupport() !== "supported" || Notification.permission !== "granted") return
    void qc
      .fetchQuery({ queryKey: pushKeys.vapid, queryFn: vapidKey, staleTime: Infinity })
      .then((key) => (key ? ensureSubscription(key) : null))
      .then((saved) => {
        if (!saved) return
        void qc.invalidateQueries({ queryKey: pushKeys.subscriptions })
        void qc.invalidateQueries({ queryKey: pushKeys.browser })
      })
      .catch(() => {})
  }, [qc])
}
