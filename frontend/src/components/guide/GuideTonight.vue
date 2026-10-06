<!--
  GuideHub's "Tonight": everything that starts in the evening's prime
  slot (PRIME_FROM–PRIME_TO in the display time zone, i.e. the 20:15
  slot and what starts around it) on the bouquet's channels, as poster
  cards with what enrichment knows — the "what do I watch tonight" view.
  A day picker looks ahead a week. Cards open the shared details dialog.
-->
<template>
  <div>
    <div class="flex flex-wrap items-center gap-3">
      <DayPicker v-model="day" />
      <span class="text-[13px] text-muted">{{ slotLabel }}</span>
    </div>

    <p v-if="loading" class="mt-5 text-[14px] text-muted">{{ t('common.loading') }}</p>
    <p v-else-if="loadFailed" class="mt-5 text-[14px] text-muted">{{ t('common.backendUnreachable') }}</p>
    <p v-else-if="shown.length === 0" class="mt-5 text-[14px] text-muted">{{ filterActive ? t('guide.filteredEmpty') : t('guide.empty') }}</p>
    <ul v-else class="mt-4 grid grid-cols-3 gap-3 below-lg:grid-cols-2 below-md:grid-cols-1">
      <li v-for="e in shown" :key="e.serviceReference + e.start">
        <ProgrammeCard
          :title="e.title"
          :service-name="e.serviceName"
          :service-reference="e.serviceReference"
          :start="e.start"
          :duration="e.duration"
          :time="clock(e.start)"
          :now="nowSec"
          :info="enrich.info(e.title)"
          :in-plex="e.inPlex"
          @open="$emit('select', e)"
        >
          {{ e.description }}
        </ProgrammeCard>
      </li>
    </ul>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import DayPicker from './DayPicker.vue'
import ProgrammeCard from '@/components/programme/ProgrammeCard.vue'
import { useEnrichStore } from '@/stores/enrich'
import { useDisplayTimeZone } from '@/composables/useDisplayTimeZone'
import { zonedUnix } from '@/lib/zonedTime'
import type { EPGEvent } from '@/api/types'

// The prime slot: programmes starting from 19:55 up to 21:30.
const PRIME_FROM: [number, number] = [19, 55]
const PRIME_TO: [number, number] = [21, 30]

const props = defineProps<{ bouquet: string; filterActive: boolean; passes: (title: string) => boolean }>()
defineEmits<{ select: [e: EPGEvent] }>()

const { t, locale } = useI18n()
const { timeZone } = useDisplayTimeZone()
const enrich = useEnrichStore()

const day = ref(0)
const events = ref<EPGEvent[]>([])
const loading = ref(true)
const loadFailed = ref(false)
const nowSec = ref(Date.now() / 1000)

function clock(s: number) {
  return new Intl.DateTimeFormat(locale.value, { timeStyle: 'short', timeZone: timeZone.value }).format(s * 1000)
}
const from = computed(() => zonedUnix(timeZone.value, day.value, ...PRIME_FROM))
const to = computed(() => zonedUnix(timeZone.value, day.value, ...PRIME_TO))
const slotLabel = computed(() => t('guide.primeSlot', { from: clock(from.value), to: clock(to.value) }))

// Long programmes first within the same start: a 90-minute film before
// the 5-minute news flash that starts with it.
const shown = computed(() =>
  events.value
    .filter((e) => e.duration >= 15 * 60 && props.passes(e.title))
    .sort((a, b) => a.start - b.start || b.duration - a.duration),
)

let token = 0
async function load() {
  if (!props.bouquet) return
  const mine = ++token
  loading.value = true
  nowSec.value = Date.now() / 1000
  try {
    const res = await api.guide.grid(props.bouquet, from.value, to.value)
    if (mine !== token) return
    events.value = res.channels.flatMap((c) => c.events)
    loadFailed.value = false
  } catch {
    if (mine === token) loadFailed.value = true
  } finally {
    if (mine === token) loading.value = false
  }
}
watch([() => props.bouquet, from], load, { immediate: true })
</script>
