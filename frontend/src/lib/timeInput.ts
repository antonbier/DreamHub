// Reading a time typed by a person: "90", "1:30" or "1:02:03" → seconds.
// null for anything else (empty, letters, minutes or seconds of 60+ after
// a colon).
export function parseClock(text: string): number | null {
  const parts = text.trim().split(':')
  if (parts.length < 1 || parts.length > 3 || parts.some((p) => !/^\d{1,5}$/.test(p))) return null
  const nums = parts.map(Number)
  if (nums.length > 1 && nums.slice(1).some((n) => n >= 60)) return null
  return nums.reduce((total, n) => total * 60 + n, 0)
}

// The box's "length" of a recording ("43:20" = minutes:seconds, or
// "1:12:05"), in seconds; 0 if it can't be read.
export function parseLength(text: string): number {
  return parseClock(text) ?? 0
}
