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
