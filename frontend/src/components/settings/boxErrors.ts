// Maps the box endpoints' error codes to translated messages.
import { ApiError } from '@/api/client'

const known: Record<string, string> = {
  box_unauthorized: 'boxes.error.unauthorized',
  box_unreachable: 'boxes.error.unreachable',
  box_not_found: 'boxes.error.notFound',
  box_is_main: 'boxes.error.isMain',
}

export function boxErrorKey(err: unknown): string {
  if (err instanceof ApiError) {
    if (known[err.code]) return known[err.code]
    if (err.code === 'validation_failed') {
      const fields = err.details ?? {}
      if (fields.url) return 'boxes.error.url'
      if (fields.name) return 'boxes.error.name'
    }
  }
  return 'boxes.error.generic'
}
