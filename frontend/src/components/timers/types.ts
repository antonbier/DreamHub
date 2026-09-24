import type { Plan } from '@/api/types'

// The delete preview of one timer row: loading (busy, no plan yet), the
// previewed plan, applying (busy with a plan), or an error to show.
export interface DeleteState {
  key: string
  plan?: Plan
  busy: boolean
  error?: string
}
