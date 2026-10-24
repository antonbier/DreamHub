// An internet channel (docs/NETCAST_PLAN.md, N2) is a bouquet entry that
// enigma2 plays through its own player instead of a tuner: a 4097 (or 8193)
// reference with a stream address, or one of Netcast's placeholders. It has no
// tuner, so it cannot be recorded. Mirrors backend iptv.StreamKind.
export function isInternetChannel(ref: string): boolean {
  const parts = ref.split(':')
  if (parts.length < 11 || (parts[0] !== '4097' && parts[0] !== '8193')) return false
  const tail = parts[10].toLowerCase()
  if (tail === '') return false
  // a recording played from the disk is not an internet channel
  return !(tail.startsWith('/') || tail.startsWith('%2f') || tail.startsWith('file%3a'))
}
