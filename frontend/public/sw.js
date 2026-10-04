// DreamHub's service worker: makes the app installable and lets the shell
// open without a round trip. Only the app itself is cached — never
// /api/*: box data is always live, and an unreachable box shows as such
// in the status line instead of stale timers or recordings.
//
// - Pages (navigations): network first, the cached shell when the network
//   fails, so the app still opens and can say the box is unreachable.
// - /assets/*: file names carry a content hash, so cache first is safe.
// - Icons and the manifest: cached copy now, refreshed in the background.
//
// Registered by src/lib/pwa.ts, only in a secure context (HTTPS or
// localhost) — browsers refuse service workers on plain http otherwise.
const CACHE = 'dreamhub-shell-v1'

// The shell plus every /assets/ file it references (entry script, CSS,
// preloaded chunks): the first visit happens before this worker controls
// the page, so without this the cached shell would open offline unstyled.
// Chunks of screens not referenced here are cached once they're visited.
async function precacheShell() {
  const cache = await caches.open(CACHE)
  const res = await fetch('/', { cache: 'no-cache' })
  if (!res.ok) return
  const html = await res.clone().text()
  await cache.put('/', res)
  const assets = [...new Set(Array.from(html.matchAll(/(?:src|href)="(\/assets\/[^"]+)"/g), (m) => m[1]))]
  await cache.addAll(assets)
}

self.addEventListener('install', (event) => {
  event.waitUntil(precacheShell().then(() => self.skipWaiting()))
})

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) => Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k))))
      .then(() => self.clients.claim()),
  )
})

self.addEventListener('fetch', (event) => {
  const req = event.request
  const url = new URL(req.url)
  if (req.method !== 'GET' || url.origin !== self.location.origin || url.pathname.startsWith('/api/')) return

  if (req.mode === 'navigate') {
    event.respondWith(
      fetch(req)
        .then((res) => {
          if (res.ok) {
            const copy = res.clone()
            caches.open(CACHE).then((c) => c.put('/', copy))
          }
          return res
        })
        .catch(() => caches.match('/').then((cached) => cached || Response.error())),
    )
    return
  }

  if (url.pathname.startsWith('/assets/')) {
    event.respondWith(
      caches.match(req).then(
        (cached) =>
          cached ||
          fetch(req).then((res) => {
            if (res.ok) {
              const copy = res.clone()
              caches.open(CACHE).then((c) => c.put(req, copy))
            }
            return res
          }),
      ),
    )
    return
  }

  if (url.pathname.startsWith('/icons/') || url.pathname === '/manifest.webmanifest') {
    event.respondWith(
      caches.match(req).then((cached) => {
        const fresh = fetch(req)
          .then((res) => {
            if (res.ok) {
              const copy = res.clone()
              caches.open(CACHE).then((c) => c.put(req, copy))
            }
            return res
          })
          .catch(() => cached)
        return cached || fresh
      }),
    )
  }
})
