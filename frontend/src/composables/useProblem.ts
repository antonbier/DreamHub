// Translates status.Problem values from the backend (internal/status,
// "Code and Fix are i18n keys... never finished text") into UI strings.
// `problem.<code>` and `fix.<fix>` are namespaced the same way the backend
// codes are (dots become nested JSON keys in the locale files).
import { useI18n } from 'vue-i18n'
import type { Problem } from '@/api/types'
import { useHubLabel } from './useHubLabel'

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
  const { t, te } = useI18n()
  const { hubName } = useHubLabel()

  // The backend names hubs by id ("timer", or "media, timer"); the screen
  // says what they are called there ("Timers").
  function withHubNames(params: Record<string, unknown> | undefined): Record<string, unknown> {
    const out = { ...(params ?? {}) }
    if (typeof out.hub === 'string') out.hub = hubName(out.hub)
    // A settings section id ("tmdb") as the section's title on screen.
    if (typeof out.section === 'string' && te(`settings.${out.section}.title`)) out.section = t(`settings.${out.section}.title`)
    if (typeof out.hubs === 'string') out.hubs = out.hubs.split(', ').map(hubName).join(', ')
    return out
  }

  // problem.Code is unprefixed (e.g. "connector.notConfigured" — status.go)
  // and lives under the "problem" locale namespace here.
  // The optional film database being off is not an error the household
  // needs the technical wording for.
  const isTmdbOff = (p: Problem) => p.code === 'connector.notConfigured' && p.params?.connector === 'TMDB'

  function describe(problem: Problem): string {
    if (isTmdbOff(problem)) return t('problem.tmdbOff')
    return translateOrRaw(t, `problem.${problem.code}`, problem.code, withHubNames(problem.params))
  }

  // problem.Fix, unlike Code, already carries its own "fix." prefix in the
  // backend (status.go: FixConfigureInSettings = "fix.configureInSettings"),
  // so it's used as the locale key directly, not prefixed again.
  function describeFix(problem: Problem): string | undefined {
    if (!problem.fix || isTmdbOff(problem)) return undefined
    return translateOrRaw(t, problem.fix, problem.fix, withHubNames(problem.fixParams))
  }

  return { describe, describeFix }
}
