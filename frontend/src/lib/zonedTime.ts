// Wall-clock times in the display time zone (useDisplayTimeZone), for
// views that ask for "today at 20:00" or "tomorrow's TV day" — the
// browser's own zone may differ from the one the user reads times in.

// The zone's offset from UTC at a moment, in milliseconds.
function offsetAt(ms: number, timeZone: string | undefined): number {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone,
    hourCycle: 'h23',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).formatToParts(ms)
  const get = (type: string) => Number(parts.find((p) => p.type === type)?.value)
  const asUTC = Date.UTC(get('year'), get('month') - 1, get('day'), get('hour'), get('minute'), get('second'))
  return asUTC - Math.floor(ms / 1000) * 1000
}

// dayKey is a moment's date in the zone, as YYYY-MM-DD.
export function dayKey(unixSeconds: number, timeZone: string | undefined): string {
  return new Intl.DateTimeFormat('en-CA', { year: 'numeric', month: '2-digit', day: '2-digit', timeZone }).format(unixSeconds * 1000)
}

// zonedUnix is the Unix time of hh:mm in the zone, dayOffset days after
// the zone's today (DST-safe: the offset is taken at the target time).
export function zonedUnix(timeZone: string | undefined, dayOffset: number, hh: number, mm: number, nowMs = Date.now()): number {
  const [y, m, d] = dayKey(nowMs / 1000, timeZone).split('-').map(Number)
  const guess = Date.UTC(y, m - 1, d + dayOffset, hh, mm)
  let t = guess - offsetAt(guess, timeZone)
  t = guess - offsetAt(t, timeZone)
  return Math.floor(t / 1000)
}
