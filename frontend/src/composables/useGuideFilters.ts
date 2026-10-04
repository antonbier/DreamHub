// GuideHub's two filters across every view: "films only" and "rated 7+",
// both from enrichment (useEnrichStore), remembered per browser. passes()
// asks enrichment about a title the first time it's checked; until the
// answer is in (or when nothing is known) a title doesn't pass — a filter
// shows what it can vouch for. With no filter on, everything passes and
// nothing is looked up.
import { computed, ref, watch } from 'vue'
import { useEnrichStore } from '@/stores/enrich'

const KEY = 'dreamhub.guideFilters'
const MIN_RATING = 7
const MIN_VOTES = 5

function read(): { movies: boolean; rated: boolean } {
  try {
    const v = JSON.parse(localStorage.getItem(KEY) ?? '{}')
    return { movies: !!v.movies, rated: !!v.rated }
  } catch {
    return { movies: false, rated: false }
  }
}

export function useGuideFilters() {
  const enrich = useEnrichStore()
  const initial = read()
  const moviesOnly = ref(initial.movies)
  const topRated = ref(initial.rated)
  watch([moviesOnly, topRated], ([movies, rated]) => {
    try {
      localStorage.setItem(KEY, JSON.stringify({ movies, rated }))
    } catch {
      // Not remembered; still applies now.
    }
  })
  const active = computed(() => moviesOnly.value || topRated.value)

  function passes(title: string): boolean {
    if (!active.value) return true
    const info = enrich.info(title)
    if (!info) return false
    if (moviesOnly.value && info.kind !== 'movie') return false
    if (topRated.value && !((info.rating ?? 0) >= MIN_RATING && (info.votes ?? 0) >= MIN_VOTES)) return false
    return true
  }

  return { moviesOnly, topRated, active, passes }
}
