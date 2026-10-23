<!--
  Other airings of one programme, inside its dialog: the box's own
  "similar broadcasts" search (GET /api/v1/guide/similar — repeats of the
  same event, not the next episode). Each one opens in the same dialog
  (select), where it can be recorded instead. Shows nothing while loading,
  when the box has no such search or finds nothing, so the dialog doesn't
  jump for programmes that never repeat. The first few, then "show all".
-->
<template>
  <section v-if="events.length" class="mt-4">
    <h3 class="text-[12.5px] text-faint">{{ t('guide.repeats.title') }}</h3>
    <ul class="mt-1 flex flex-col">
      <li v-for="e in shown" :key="`${e.serviceReference}|${e.id}`">
        <button
          type="button"
          class="flex w-full items-baseline gap-2 rounded-sm px-1.5 py-1 text-start text-[13.5px] hover:bg-field"
          @click="$emit('select', e)"
        >
          <span class="tabular-nums text-text">{{ format(e.start, e.duration) }}</span>
          <span class="min-w-0 truncate text-muted">· {{ e.serviceName }}</span>
        </button>
      </li>
    </ul>
    <button
      v-if="events.length > shown.length"
      type="button"
      class="mt-1 px-1.5 text-[13px] text-accent hover:underline"
      @click="all = true"
    >
      {{ t('guide.repeats.showAll', { n: events.length }) }}
    </button>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import type { EPGEvent } from '@/api/types'

const props = defineProps<{
  event: EPGEvent
  // The caller's day-and-time line, so it reads like the dialog's own.
  format: (start: number, duration: number) => string
}>()
defineEmits<{ select: [e: EPGEvent] }>()
const { t } = useI18n()

const FIRST = 4
const events = ref<EPGEvent[]>([])
const all = ref(false)
const shown = computed(() => (all.value ? events.value : events.value.slice(0, FIRST)))

watch(
  () => `${props.event.serviceReference}|${props.event.id}`,
  async (key) => {
    events.value = []
    all.value = false
    if (!props.event.id) return // no EPG id: nothing to ask the box for
    try {
      const res = await api.guide.similar(props.event.serviceReference, props.event.id)
      if (key === `${props.event.serviceReference}|${props.event.id}`) events.value = res.events
    } catch {
      // Repeats are extra: a failed ask leaves the dialog as it was.
    }
  },
  { immediate: true },
)
</script>
