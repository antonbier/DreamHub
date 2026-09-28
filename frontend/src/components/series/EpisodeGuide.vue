<!--
  The episode guide on a series' page (backend series_episodes.go): one
  collapsible block per season, newest first and open, with a one-line
  tally ("8 of 10 recorded · 2 missing") and one row per episode — code,
  title, first air date, and its state as a dot plus a word (recorded `ok`,
  planned `accent`, missing `warn`, still to come / aired `faint`), never
  color alone. When the recordings carry no episode codes, nothing is
  called missing; a line says why. Credits TVmaze as the source.
-->
<template>
  <section>
    <h2 class="text-[15.5px] font-semibold text-text">{{ t('series.guide.title') }}</h2>
    <p v-if="guide.state === 'off'" class="mt-2 text-[13.5px] text-muted">{{ t('series.guide.off') }}</p>
    <p v-else-if="guide.state === 'noMatch'" class="mt-2 text-[13.5px] text-muted">{{ t('series.guide.noMatch') }}</p>
    <p v-else-if="guide.state === 'failed'" class="mt-2 text-[13.5px] text-muted">{{ t('series.detail.partUnavailable') }}</p>
    <template v-else>
      <p v-if="!guide.matchable" class="mt-1 text-[12.5px] text-faint">{{ t('series.guide.notMatchable') }}</p>
      <div v-for="(s, i) in guide.seasons" :key="s.season" class="mt-3 border-b border-line-soft pb-2 last:border-none">
        <button
          type="button"
          class="flex w-full items-center gap-2 rounded py-1 text-start hover:bg-tile-2"
          :aria-expanded="isOpen(s.season, i)"
          @click="toggle(s.season, i)"
        >
          <Icon :name="isOpen(s.season, i) ? 'chevron-down' : 'chevron-right'" :size="14" class="text-faint rtl:rotate-180" />
          <span class="text-[14px] font-semibold text-text">{{ t('series.detail.season', { season: s.season }) }}</span>
          <span class="text-[12.5px] text-muted">{{ tally(s) }}</span>
        </button>
        <ul v-if="isOpen(s.season, i)" class="mt-1 flex flex-col">
          <li v-for="e in s.episodes" :key="e.number" class="flex items-center gap-3 py-1.5 ps-6">
            <span class="w-16 shrink-0 font-mono text-[12.5px] tabular-nums text-faint">{{ code(e) }}</span>
            <span class="min-w-0 flex-1 truncate text-[13.5px] text-text">{{ e.name }}</span>
            <span class="w-24 shrink-0 text-end text-[12.5px] tabular-nums text-faint below-sm:hidden">{{ date(e) }}</span>
            <span class="flex w-28 shrink-0 items-center gap-1.5 text-[12.5px]" :class="STATUS[e.status].text">
              <span class="h-1.5 w-1.5 shrink-0 rounded-full" :class="STATUS[e.status].dot" />
              {{ t(`series.guide.status.${e.status}`) }}
            </span>
          </li>
        </ul>
      </div>
      <p class="mt-2 text-[11.5px] text-faint">{{ t('series.guide.source', { source: 'TVmaze' }) }}</p>
    </template>
  </section>
</template>

<script setup lang="ts">
import { reactive } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/Icon.vue'
import type { EpisodeGuide, EpisodeStatus, GuideEpisode, GuideSeason } from '@/api/types'

const props = defineProps<{ guide: EpisodeGuide }>()
const { t, locale } = useI18n()

const STATUS: Record<EpisodeStatus, { dot: string; text: string }> = {
  recorded: { dot: 'bg-ok', text: 'text-ok' },
  planned: { dot: 'bg-accent', text: 'text-accent' },
  missing: { dot: 'bg-warn', text: 'text-warn' },
  upcoming: { dot: 'bg-faint', text: 'text-muted' },
  aired: { dot: 'bg-faint', text: 'text-faint' },
}

// The newest season starts open, the rest closed; the user's toggles win.
const toggled = reactive<Record<number, boolean>>({})
const isOpen = (season: number, index: number) => toggled[season] ?? index === 0
const toggle = (season: number, index: number) => (toggled[season] = !isOpen(season, index))

function tally(s: GuideSeason) {
  const aired = s.episodes.length - s.upcoming
  // Without episode codes, "0 of 10 recorded" would be a false claim.
  const parts = [
    props.guide.matchable
      ? t('series.guide.tally.recorded', { n: s.recorded, of: aired })
      : t('series.guide.tally.aired', { n: aired }, aired),
  ]
  if (s.missing) parts.push(t('series.guide.tally.missing', { n: s.missing }, s.missing))
  if (s.planned) parts.push(t('series.guide.tally.planned', { n: s.planned }, s.planned))
  if (s.upcoming) parts.push(t('series.guide.tally.upcoming', { n: s.upcoming }, s.upcoming))
  return parts.join(' · ')
}

function code(e: GuideEpisode) {
  return `S${String(e.season).padStart(2, '0')}E${String(e.number).padStart(2, '0')}`
}

// A first air date is a calendar date, not an instant: shown as that date.
function date(e: GuideEpisode) {
  if (!e.airdate) return ''
  const [y, m, d] = e.airdate.split('-').map(Number)
  return new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium', timeZone: 'UTC' }).format(Date.UTC(y, m - 1, d))
}
</script>
