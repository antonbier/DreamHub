import type { Plan } from '@/api/types'

// Where "Create timer" stands for one recommendation.
export type TimerState = { status: 'previewing' | 'created' | 'error'; plan?: Plan; error?: string; busy?: boolean }
