// Maps the profile endpoints' error codes to translated messages.
import { ApiError } from '@/api/client'

const known: Record<string, string> = {
  profile_admin_pin_required: 'profiles.error.adminPinRequired',
  profile_last_admin: 'profiles.error.lastAdmin',
  profile_self: 'profiles.error.self',
  profile_not_found: 'profiles.error.notFound',
}

export function profileErrorKey(err: unknown): string {
  if (err instanceof ApiError) {
    if (known[err.code]) return known[err.code]
    if (err.code === 'validation_failed') {
      const fields = err.details ?? {}
      if (fields.name?.code === 'taken') return 'profiles.error.nameTaken'
      if (fields.name) return 'profiles.error.name'
      if (fields.pin) return 'profiles.pinFormat'
    }
  }
  return 'profiles.error.generic'
}
