// Which box this page is about (docs/MULTIDEVICE_PLAN.md, M3).
//
// The box is part of the address: /b/<id>/guide is the guide of that box,
// and plain /guide is the first box the profile may use — so a household
// with one box never sees anything new. The router is given /b/<id> as its
// base (router/index.ts), which carries the prefix through every link
// without a single route knowing about it. Changing the box loads the page
// again at the new address: no store, event stream or cached answer of the
// previous box can leak into the next one.
//
// The last chosen box is remembered per device (localStorage); an address
// without a box opens it.
const REMEMBER_KEY = 'dreamhub.box'
const PREFIX = /^\/b\/([^/]+)/

/** The box named by the address, '' for none (the first box). */
export const activeBoxId: string = (() => {
  const m = PREFIX.exec(window.location.pathname)
  if (!m) return ''
  try {
    return decodeURIComponent(m[1])
  } catch {
    return ''
  }
})()

/** The router's base: '/b/<id>' or ''. */
export const routerBase: string = activeBoxId ? `/b/${encodeURIComponent(activeBoxId)}` : ''

/** The address of `path` (without any box prefix) on a box; '' is the first box. */
export function boxUrl(id: string, path: string): string {
  return (id ? `/b/${encodeURIComponent(id)}` : '') + (path.startsWith('/') ? path : `/${path}`)
}

/** This page's address without the box prefix: what the router calls the full path. */
export function pathWithoutBox(): string {
  const { pathname, search, hash } = window.location
  const rest = routerBase && pathname.startsWith(routerBase) ? pathname.slice(routerBase.length) || '/' : pathname
  return rest + search + hash
}

export function rememberedBox(): string {
  try {
    return localStorage.getItem(REMEMBER_KEY) ?? ''
  } catch {
    return ''
  }
}

/** Remembers the box for this device; '' (the first box) forgets. */
export function rememberBox(id: string) {
  try {
    if (id) localStorage.setItem(REMEMBER_KEY, id)
    else localStorage.removeItem(REMEMBER_KEY)
  } catch {
    // Private mode: the address still says which box.
  }
}

// Questions about the door and the profile, not about a box: they must work
// even when the remembered box is not this profile's (the box store then
// notices and goes back to the first box).
const BOXLESS = ['/api/v1/health', '/api/v1/whoami', '/api/v1/profiles']

/** Adds the box parameter to an API path when the page is about another box than the first. */
export function withBox(path: string): string {
  if (!activeBoxId || !path.startsWith('/api/') || BOXLESS.some((p) => path === p || path.startsWith(p + '/') || path.startsWith(p + '?'))) return path
  return `${path}${path.includes('?') ? '&' : '?'}box=${encodeURIComponent(activeBoxId)}`
}

/**
 * Called before the app starts: an address without a box opens the box
 * remembered on this device. True when the page is being replaced.
 */
export function openRememberedBox(): boolean {
  if (activeBoxId) return false
  const remembered = rememberedBox()
  if (!remembered) return false
  window.location.replace(boxUrl(remembered, pathWithoutBox()))
  return true
}
