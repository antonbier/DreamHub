// What further databases add to a title already identified (GET
// /api/v1/enrich/extras): its logo from fanart.tv, and Rotten Tomatoes'
// Tomatometer, IMDb's rating and Metacritic's Metascore from OMDb, as
// chips with Intl-formatted numbers. Asked once per title per session
// (cached promises), only for a detail view. Empty while loading or when
// nothing is known, so a screen without these keys looks as before.
import { computed, ref, watch, type Ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import type { EnrichExtras, EnrichInfo } from '@/api/types'

const extrasCache = new Map<string, Promise<EnrichExtras | null>>()
const logoCache = new Map<string, Promise<string | null>>()

function cached<T>(cache: Map<string, Promise<T | null>>, key: string, load: () => Promise<T>) {
  let p = cache.get(key)
  if (!p) {
    p = load().catch(() => {
      cache.delete(key) // try again another time
      return null
    })
    cache.set(key, p)
  }
  return p
}

export interface ExtrasChip {
  id: string
  label: string
  value: string
  title: string
}

export function useEnrichExtras(info: Ref<EnrichInfo | null | undefined>) {
  const { t, locale } = useI18n()
  const extras = ref<EnrichExtras | null>(null)
  const logoSrc = ref<string | null>(null)

  watch(
    () => (info.value && (info.value.kind === 'movie' || info.value.kind === 'tv') && info.value.id ? `${info.value.kind}:${info.value.id}` : ''),
    async (key) => {
      extras.value = null
      logoSrc.value = null
      if (!key) return
      const [kind, id] = key.split(':') as ['movie' | 'tv', string]
      const x = await cached(extrasCache, key, () => api.enrich.extras(kind, Number(id)).then((r) => r.extras))
      if (!x || currentKey() !== key) return
      extras.value = x
      if (x.logo) {
        const logo = x.logo
        const url = await cached(logoCache, logo, () => api.enrich.logo(logo).then((b) => URL.createObjectURL(b)))
        if (currentKey() === key) logoSrc.value = url
      }
    },
    { immediate: true },
  )
  const currentKey = () => (info.value ? `${info.value.kind}:${info.value.id}` : '')

  const chips = computed<ExtrasChip[]>(() => {
    const x = extras.value
    if (!x) return []
    const n = (v: number, digits = 0) =>
      new Intl.NumberFormat(locale.value, { minimumFractionDigits: digits, maximumFractionDigits: digits }).format(v)
    const out: ExtrasChip[] = []
    if (x.rottenTomatoes) {
      const v = new Intl.NumberFormat(locale.value, { style: 'percent' }).format(x.rottenTomatoes / 100)
      out.push({ id: 'rt', label: 'Rotten Tomatoes', value: v, title: t('enrich.extras.rottenTomatoes', { value: v }) })
    }
    if (x.imdb) {
      const votes = new Intl.NumberFormat(locale.value, { notation: 'compact' }).format(x.imdbVotes ?? 0)
      out.push({ id: 'imdb', label: 'IMDb', value: n(x.imdb, 1), title: t('enrich.extras.imdb', { value: n(x.imdb, 1), votes }) })
    }
    if (x.metascore) out.push({ id: 'meta', label: 'Metascore', value: n(x.metascore), title: t('enrich.extras.metascore', { value: n(x.metascore) }) })
    return out
  })

  // Further sources, for the credit line.
  const sources = computed(() => {
    const out: string[] = []
    if (chips.value.length) out.push('OMDb')
    if (logoSrc.value) out.push('fanart.tv')
    return out
  })

  return { logoSrc, chips, sources }
}
