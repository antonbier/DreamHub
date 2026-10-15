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
    // 1.5 MB, 830 MB, 1.2 GB: a decimal only where the number is small or big units.
    maximumFractionDigits: unit >= 3 || (unit >= 1 && value < 10) ? 1 : 0,
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

// Whole minutes ("14 min", "14 Min."), rounded, per locale.
export function formatMinutes(seconds: number, locale: string): string {
  return new Intl.NumberFormat(locale, { style: 'unit', unit: 'minute', unitDisplay: 'short', maximumFractionDigits: 0 }).format(
    Math.round(seconds / 60),
  )
}

// A position inside a recording, h:mm:ss or m:ss — a clock reading, the
// same in every language.
export function formatPosition(seconds: number): string {
  const s = Math.max(0, Math.floor(seconds))
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const ss = String(s % 60).padStart(2, '0')
  return h > 0 ? `${h}:${String(m).padStart(2, '0')}:${ss}` : `${m}:${ss}`
}
