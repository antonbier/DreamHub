import type { Plan } from '@/api/types'

// The pending action of one timer row — delete, disable or enable:
// loading (busy, no plan yet), the previewed plan, applying (busy with a
// plan), or an error to show.
export interface DeleteState {
  key: string
  kind: 'delete' | 'disable' | 'enable'
  plan?: Plan
  busy: boolean
  error?: string
}
