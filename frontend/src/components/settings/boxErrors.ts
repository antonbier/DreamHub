// Maps the box endpoints' error codes to translated messages.
import { ApiError } from '@/api/client'

const known: Record<string, string> = {
  box_unauthorized: 'boxes.error.unauthorized',
  box_unreachable: 'boxes.error.unreachable',
  box_not_found: 'boxes.error.notFound',
  box_is_main: 'boxes.error.isMain',
  box_is_local: 'boxes.error.isLocal',
  peer_unreachable: 'boxes.error.peerUnreachable',
  peer_not_found: 'boxes.error.peerNotFound',
  peer_not_pairing: 'boxes.error.peerNotPairing',
  peer_bad_code: 'boxes.error.peerBadCode',
}

export function boxErrorKey(err: unknown): string {
  if (err instanceof ApiError) {
    if (known[err.code]) return known[err.code]
    if (err.code === 'peer_version_mismatch') {
      return err.details?.compat?.code === 'hub_too_old' ? 'boxes.error.hubTooOld' : 'boxes.error.boxTooOld'
    }
    if (err.code === 'validation_failed') {
      if (err.details?.code) return 'boxes.error.codeRequired'
      const fields = err.details ?? {}
      if (fields.url) return 'boxes.error.url'
      if (fields.name) return 'boxes.error.name'
    }
  }
  return 'boxes.error.generic'
}
