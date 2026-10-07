// A minimal Server-Sent Events reader over fetch(), not the browser's
// native EventSource — DreamHub's own Basic Auth header (api/authState.ts)
// isn't something EventSource can send (it can't set custom headers at
// all), so this reads the stream itself instead. Backed by
// GET /api/v1/events/stream (backend/internal/api's handleEventsStream) and
// GET /api/v1/admin/logs/stream, both plain "server pushes lines" streams —
// see that handler's own doc comment for why SSE instead of a real
// WebSocket. Auto-reconnects with a fixed backoff; call the returned
// function to stop for good.
import { authHeaders } from '@/api/authState'

export interface StreamEvent {
  event: string
  data: string
}

const RECONNECT_DELAY_MS = 3000

function parseChunk(chunk: string): StreamEvent | null {
  let event = 'message'
  const dataLines: string[] = []
  for (const line of chunk.split('\n')) {
    if (line.startsWith('event:')) event = line.slice(6).trim()
    else if (line.startsWith('data:')) dataLines.push(line.slice(5).trim())
    // Lines starting with ":" are comments (the stream's keep-alive pings) — ignored.
  }
  if (dataLines.length === 0) return null
  return { event, data: dataLines.join('\n') }
}

/** Opens path as an SSE stream, calling onEvent for each event. Returns a stop function. */
export function openEventStream(path: string, onEvent: (e: StreamEvent) => void): () => void {
  const controller = new AbortController()
  let stopped = false

  async function connectOnce() {
    const headers: Record<string, string> = { Accept: 'text/event-stream' }
    Object.assign(headers, authHeaders())
    const res = await fetch(path, { headers, signal: controller.signal })
    if (!res.ok || !res.body) throw new Error(`event stream ${path}: ${res.status}`)
    const reader = res.body.getReader()
    const decoder = new TextDecoder()
    let buf = ''
    for (;;) {
      const { done, value } = await reader.read()
      if (done) return
      buf += decoder.decode(value, { stream: true })
      let idx: number
      while ((idx = buf.indexOf('\n\n')) >= 0) {
        const evt = parseChunk(buf.slice(0, idx))
        buf = buf.slice(idx + 2)
        if (evt) onEvent(evt)
      }
    }
  }

  async function loop() {
    while (!stopped) {
      try {
        await connectOnce()
      } catch {
        // Network hiccup, box restart, or a 401 — either way, back off and
        // try again rather than giving up the live view silently.
      }
      if (stopped) return
      await new Promise((r) => setTimeout(r, RECONNECT_DELAY_MS))
    }
  }
  void loop()

  return () => {
    stopped = true
    controller.abort()
  }
}
