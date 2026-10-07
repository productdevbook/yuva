const VERSION = self.__YUVA_VERSION__
const PRECACHE = self.__YUVA_PRECACHE__
const CACHE = `yuva-${VERSION}`
const NETWORK_ONLY = ["/v1/", "/client/v1/", "/ingress/", "/healthz", "/readyz", "/yuva.js", "/yuva-chat.js"]

self.addEventListener("install", (event) => {
  event.waitUntil(caches.open(CACHE).then((cache) => cache.addAll(PRECACHE)))
})

self.addEventListener("activate", (event) => {
  event.waitUntil(
    (async () => {
      for (const key of await caches.keys()) {
        if (key.startsWith("yuva-") && key !== CACHE) await caches.delete(key)
      }
      await self.clients.claim()
    })(),
  )
})

self.addEventListener("message", (event) => {
  if (event.data?.type === "skip-waiting") self.skipWaiting()
})

self.addEventListener("fetch", (event) => {
  const request = event.request
  if (request.method !== "GET") return
  const url = new URL(request.url)
  if (url.origin !== self.location.origin) return
  if (NETWORK_ONLY.some((p) => url.pathname.startsWith(p))) return
  if (request.mode === "navigate") {
    event.respondWith(fetch(request).catch(() => caches.match("/index.html", { cacheName: CACHE })))
    return
  }
  event.respondWith(
    caches.match(request, { cacheName: CACHE, ignoreSearch: true }).then((hit) => hit ?? fetch(request)),
  )
})

self.addEventListener("push", (event) => {
  let payload = {}
  try {
    payload = event.data?.json() ?? {}
  } catch {
    payload = { body: event.data?.text() }
  }
  const options = {
    body: payload.body ?? "",
    icon: "/icons/icon-192.png",
    badge: "/icons/badge-96.png",
    data: { url: payload.url ?? "/" },
  }
  if (payload.tag) {
    options.tag = payload.tag
    options.renotify = true
  }
  event.waitUntil(self.registration.showNotification(payload.title || "Yuva", options))
})

self.addEventListener("notificationclick", (event) => {
  event.notification.close()
  const target = new URL(event.notification.data?.url ?? "/", self.location.origin)
  if (target.origin !== self.location.origin) return
  event.waitUntil(
    (async () => {
      const windows = await self.clients.matchAll({ type: "window", includeUncontrolled: true })
      const panel = windows.find((c) => c.focused) ?? windows[0]
      if (panel) {
        panel.postMessage({ type: "navigate", url: target.pathname + target.search + target.hash })
        await panel.focus().catch(() => {})
        return
      }
      await self.clients.openWindow(target.href)
    })(),
  )
})

self.addEventListener("pushsubscriptionchange", (event) => {
  event.waitUntil(
    (async () => {
      let sub = event.newSubscription
      if (!sub) {
        const res = await fetch("/v1/push/vapid-public-key", { credentials: "same-origin" })
        if (!res.ok) return
        const { public_key: key } = await res.json()
        sub = await self.registration.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: key })
      }
      const json = sub.toJSON()
      await fetch("/v1/me/push-subscriptions", {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ endpoint: sub.endpoint, keys: json.keys }),
      })
    })(),
  )
})
