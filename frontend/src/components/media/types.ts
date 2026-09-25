import type { NameSuggestion, Plan, SuggestionSource } from '@/api/types'

// The inline action of one recording row: the rename form or a preview
// (busy while loading/applying, error when something was refused).
export interface RowState {
  kind: 'rename' | 'trash'
  plan?: Plan
  busy: boolean
  error?: string
  // Name suggestions while renaming: the source being asked, its answer.
  suggesting?: SuggestionSource
  suggestions?: NameSuggestion[]
  suggestError?: string
}

// Which suggestion sources can be asked (TMDB and AI need their keys).
export interface SuggestSources {
  tmdb: boolean
  ai: boolean
}

// A trash action being previewed: restore (rename back) or delete for good.
// `all` marks the "empty the whole trash" variant, shown above the list.
export interface TrashActionState {
  kind: 'restore' | 'empty'
  paths: string[]
  all: boolean
  plan?: Plan
  bytes?: number
  busy: boolean
  error?: string
}
