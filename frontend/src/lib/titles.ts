// Title comparison for display decisions, matching the backend's
// internal/titles normalization (case, accents and punctuation ignored).
export function normalizeTitle(s: string): string {
  return s
    .toLowerCase()
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .replace(/[^\p{L}\p{N}]+/gu, ' ')
    .trim()
}

// titlesDiffer is true when a is worth showing next to b.
export function titlesDiffer(a: string, b?: string): boolean {
  return !b || normalizeTitle(a) !== normalizeTitle(b)
}

// A recording without a .ts.meta title shows Enigma2's default file name
// ("YYYYMMDD HHMM - Channel - Title.ts"); this is its title part, with the
// "_ " Enigma2 writes for a colon turned back into ": " (matches the
// backend's titles.FromFileName). Only the title — never a time.
export function titleFromFileName(s: string): string {
  const m = /^\d{8} \d{4} - [^-]+? - (.+?)(?:\.ts)?$/.exec(s.trim())
  return m ? m[1].replaceAll('_ ', ': ') : s
}
