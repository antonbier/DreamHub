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
