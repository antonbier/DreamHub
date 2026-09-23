// Translates status.Problem values from the backend (internal/status,
// "Code and Fix are i18n keys... never finished text") into UI strings.
// `problem.<code>` and `fix.<fix>` are namespaced the same way the backend
// codes are (dots become nested JSON keys in the locale files).
import { useI18n } from 'vue-i18n'
import type { Problem } from '@/api/types'

// `te()` checks the active locale only, not the English fallback, so a
// problem/fix code missing from say Italian would wrongly report "doesn't
// exist" even though it resolves fine. Comparing the translated result to
// the key itself (vue-i18n's behavior for a truly missing message) sidesteps
// that and matches the fallback chain the UI actually uses.
function translateOrRaw(t: (key: string, params: Record<string, unknown>) => string, key: string, raw: string, params: Record<string, unknown>) {
  const result = t(key, params)
  return result === key ? raw : result
}

export function useProblem() {
  const { t } = useI18n()

  // problem.Code is unprefixed (e.g. "connector.notConfigured" — status.go)
  // and lives under the "problem" locale namespace here.
  function describe(problem: Problem): string {
    return translateOrRaw(t, `problem.${problem.code}`, problem.code, problem.params ?? {})
  }

  // problem.Fix, unlike Code, already carries its own "fix." prefix in the
  // backend (status.go: FixConfigureInSettings = "fix.configureInSettings"),
  // so it's used as the locale key directly, not prefixed again.
  function describeFix(problem: Problem): string | undefined {
    if (!problem.fix) return undefined
    return translateOrRaw(t, problem.fix, problem.fix, problem.fixParams ?? {})
  }

  return { describe, describeFix }
}
