// Byte sizes for the UI. Decimal units (1 GB = 1000³ bytes), matching how
// Enigma2 itself reports sizes (CONCEPT.md, "Units"), formatted per locale
// through Intl — never hand-built strings (PRINCIPLES.md).
const UNITS = ['byte', 'kilobyte', 'megabyte', 'gigabyte', 'terabyte'] as const

export function formatBytes(bytes: number, locale: string): string {
  let value = bytes
  let unit = 0
  while (value >= 1000 && unit < UNITS.length - 1) {
    value /= 1000
    unit++
  }
  return new Intl.NumberFormat(locale, {
    style: 'unit',
    unit: UNITS[unit],
    unitDisplay: 'short',
    maximumFractionDigits: unit >= 3 ? 1 : 0,
  }).format(value)
}

// A start time for a short list that may span several days: just the time
// when it's today, the weekday too within the coming week, else the date —
// so "08:52" never silently means "the day after tomorrow". Days are
// compared in the display time zone, not the browser's.
export function formatWhen(unixSeconds: number, locale: string, timeZone: string | undefined, now = Date.now()): string {
  const day = new Intl.DateTimeFormat('en-CA', { year: 'numeric', month: '2-digit', day: '2-digit', timeZone })
  const at = unixSeconds * 1000
  const time: Intl.DateTimeFormatOptions = { hour: '2-digit', minute: '2-digit', timeZone }
  if (day.format(at) === day.format(now)) return new Intl.DateTimeFormat(locale, time).format(at)
  if (at > now && at - now < 6 * 86400_000) return new Intl.DateTimeFormat(locale, { ...time, weekday: 'short' }).format(at)
  return new Intl.DateTimeFormat(locale, { ...time, day: 'numeric', month: 'short' }).format(at)
}
